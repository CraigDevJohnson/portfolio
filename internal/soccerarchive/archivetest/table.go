// Package archivetest provides an in-memory DynamoDB table for exercising the
// durable Soccer archive without AWS.
package archivetest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Table stores items by pk/sk. Every put must carry one of the archive's
// condition expressions, which the table evaluates as DynamoDB would: a
// failed condition returns ConditionalCheckFailedException.
type Table struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue
	// FailPut, when set, is called with each put's "pk/sk" key; a non-nil
	// result fails that put as an unavailable or throttled table would.
	FailPut func(key string) error
	// FailDelete, when set, is called with each delete's "pk/sk" key; a
	// non-nil result fails that delete.
	FailDelete func(key string) error
	// PageSize, when positive, limits each base-table and index query page to
	// that many items and returns a LastEvaluatedKey for the rest, as
	// DynamoDB's 1 MB page does.
	PageSize     int
	indexQueries int
}

// NewTable returns an empty table.
func NewTable() *Table {
	return &Table{items: make(map[string]map[string]types.AttributeValue)}
}

type itemKey struct {
	PK string `dynamodbav:"pk"`
	SK string `dynamodbav:"sk"`
}

// PutItem implements the DynamoDB PutItem subset used by the archive.
func (t *Table) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key, err := t.checkPut(input.Item, input.ConditionExpression, input.ExpressionAttributeNames, input.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	t.items[key] = input.Item
	return &dynamodb.PutItemOutput{}, nil
}

// TransactWriteItems implements the all-or-nothing conditional puts the
// archive uses to enroll a team and take an admission slot together. A
// failed condition cancels every put, reporting each put's reason in order.
func (t *Table) TransactWriteItems(_ context.Context, input *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	keys := make([]string, len(input.TransactItems))
	reasons := make([]types.CancellationReason, len(input.TransactItems))
	canceled := false
	for i, write := range input.TransactItems {
		if write.Put == nil {
			return nil, errors.New("archivetest transactions support only conditional puts")
		}
		key, err := t.checkPut(write.Put.Item, write.Put.ConditionExpression, write.Put.ExpressionAttributeNames, write.Put.ExpressionAttributeValues)
		var failed *types.ConditionalCheckFailedException
		switch {
		case errors.As(err, &failed):
			reasons[i].Code, canceled = aws.String("ConditionalCheckFailed"), true
		case err != nil:
			return nil, err
		default:
			reasons[i].Code = aws.String("None")
		}
		keys[i] = key
	}
	if canceled {
		return nil, &types.TransactionCanceledException{Message: aws.String("Transaction canceled"), CancellationReasons: reasons}
	}
	for i, write := range input.TransactItems {
		t.items[keys[i]] = write.Put.Item
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

// GetItem implements the DynamoDB GetItem subset used by the archive.
func (t *Table) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var key itemKey
	if err := attributevalue.UnmarshalMap(input.Key, &key); err != nil {
		return nil, err
	}
	return &dynamodb.GetItemOutput{Item: t.items[key.PK+"/"+key.SK]}, nil
}

// Query implements the archive's base-table "pk = :pk" and
// "pk = :pk AND begins_with(sk, :prefix)" queries and the due-teams index's
// "due_pk = :due_pk AND due_sk <= :cutoff", in key order. Both are paged by
// PageSize from ExclusiveStartKey, as DynamoDB pages results.
func (t *Table) Query(_ context.Context, input *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if input.IndexName != nil {
		return t.queryDueTeams(input)
	}
	pk := stringValue(input.ExpressionAttributeValues[":pk"])
	prefix := stringValue(input.ExpressionAttributeValues[":prefix"])
	startSK := ""
	if len(input.ExclusiveStartKey) > 0 {
		var start itemKey
		if err := attributevalue.UnmarshalMap(input.ExclusiveStartKey, &start); err != nil {
			return nil, err
		}
		if start.PK != pk {
			return nil, fmt.Errorf("archivetest query start key %q is outside partition %q", start.PK, pk)
		}
		startSK = start.SK
	}
	keys := make([]string, 0)
	for key := range t.items {
		if strings.HasPrefix(key, pk+"/"+prefix) && (startSK == "" || strings.TrimPrefix(key, pk+"/") > startSK) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var lastKey map[string]types.AttributeValue
	if t.PageSize > 0 && len(keys) > t.PageSize {
		keys = keys[:t.PageSize]
		lastKey = map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: strings.TrimPrefix(keys[len(keys)-1], pk+"/")},
		}
	}
	items := make([]map[string]types.AttributeValue, 0, len(keys))
	for _, key := range keys {
		items = append(items, t.items[key])
	}
	return &dynamodb.QueryOutput{Items: items, LastEvaluatedKey: lastKey}, nil
}

// DeleteItem implements the DynamoDB DeleteItem subset used by the archive.
// Deleting an absent item succeeds, as it does in DynamoDB.
func (t *Table) DeleteItem(_ context.Context, input *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var key itemKey
	if err := attributevalue.UnmarshalMap(input.Key, &key); err != nil {
		return nil, err
	}
	if t.FailDelete != nil {
		if err := t.FailDelete(key.PK + "/" + key.SK); err != nil {
			return nil, err
		}
	}
	delete(t.items, key.PK+"/"+key.SK)
	return &dynamodb.DeleteItemOutput{}, nil
}

// IndexQueries reports how many index queries the table has answered.
func (t *Table) IndexQueries() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.indexQueries
}

// Item returns the stored item for "pk/sk", or nil.
func (t *Table) Item(key string) map[string]types.AttributeValue {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.items[key]
}

// Items returns every stored item decoded to plain Go values, keyed by "pk/sk".
func (t *Table) Items() (map[string]map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	decoded := make(map[string]map[string]any, len(t.items))
	for key, item := range t.items {
		var values map[string]any
		if err := attributevalue.UnmarshalMap(item, &values); err != nil {
			return nil, err
		}
		decoded[key] = values
	}
	return decoded, nil
}

// Len reports how many items are stored.
func (t *Table) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.items)
}

// checkPut evaluates a put's condition against the stored item as DynamoDB
// would and returns the item's "pk/sk" key. Every put must carry one of the
// archive's conditions; a failed one is ConditionalCheckFailedException.
func (t *Table) checkPut(item map[string]types.AttributeValue, condition *string, names map[string]string, values map[string]types.AttributeValue) (string, error) {
	var key itemKey
	if err := attributevalue.UnmarshalMap(item, &key); err != nil {
		return "", err
	}
	id := key.PK + "/" + key.SK
	if t.FailPut != nil {
		if err := t.FailPut(id); err != nil {
			return "", err
		}
	}
	if condition == nil || *condition == "" {
		return "", errors.New("archive writes must protect newer source facts")
	}
	previous := t.items[id]
	var holds bool
	switch *condition {
	case "attribute_not_exists(fetched_at) OR fetched_at <= :fetched_at":
		holds = previous == nil || fetchedAt(previous) <= stringValue(values[":fetched_at"])
	case "attribute_not_exists(pk)":
		holds = previous == nil
	case "#revision = :read_revision":
		holds = previous != nil && numberValue(previous[names["#revision"]]) == numberValue(values[":read_revision"])
	default:
		return "", fmt.Errorf("archivetest does not evaluate condition %q", *condition)
	}
	if !holds {
		return "", &types.ConditionalCheckFailedException{}
	}
	return id, nil
}

func (t *Table) queryDueTeams(input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	if aws.ToString(input.IndexName) != "due-teams" || aws.ToString(input.KeyConditionExpression) != "due_pk = :due_pk AND due_sk <= :cutoff" {
		return nil, fmt.Errorf("archivetest does not evaluate index query %q on %q", aws.ToString(input.KeyConditionExpression), aws.ToString(input.IndexName))
	}
	t.indexQueries++
	duePK := stringValue(input.ExpressionAttributeValues[":due_pk"])
	cutoff := stringValue(input.ExpressionAttributeValues[":cutoff"])
	type indexed struct{ sortKey, id string }
	matches := make([]indexed, 0)
	for id, item := range t.items {
		if stringValue(item["due_pk"]) == duePK && stringValue(item["due_sk"]) != "" && stringValue(item["due_sk"]) <= cutoff {
			matches = append(matches, indexed{sortKey: stringValue(item["due_sk"]) + "\x00" + id, id: id})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].sortKey < matches[j].sortKey })
	start := 0
	if len(input.ExclusiveStartKey) > 0 {
		after := stringValue(input.ExclusiveStartKey["due_sk"]) + "\x00" + stringValue(input.ExclusiveStartKey["pk"]) + "/" + stringValue(input.ExclusiveStartKey["sk"])
		start = sort.Search(len(matches), func(i int) bool { return matches[i].sortKey > after })
	}
	end := len(matches)
	if t.PageSize > 0 && start+t.PageSize < end {
		end = start + t.PageSize
	}
	output := &dynamodb.QueryOutput{Items: make([]map[string]types.AttributeValue, 0, end-start)}
	for _, match := range matches[start:end] {
		output.Items = append(output.Items, t.items[match.id])
	}
	if end < len(matches) {
		last := t.items[matches[end-1].id]
		output.LastEvaluatedKey = map[string]types.AttributeValue{"pk": last["pk"], "sk": last["sk"], "due_pk": last["due_pk"], "due_sk": last["due_sk"]}
	}
	return output, nil
}

func fetchedAt(item map[string]types.AttributeValue) string {
	return stringValue(item["fetched_at"])
}

func numberValue(value types.AttributeValue) string {
	if n, ok := value.(*types.AttributeValueMemberN); ok {
		return n.Value
	}
	return ""
}

func stringValue(value types.AttributeValue) string {
	if s, ok := value.(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}
