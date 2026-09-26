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
	// PageSize, when positive, limits each query page to that many items and
	// returns a LastEvaluatedKey for the rest, as DynamoDB's 1 MB page does.
	PageSize int
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
	var key itemKey
	if err := attributevalue.UnmarshalMap(input.Item, &key); err != nil {
		return nil, err
	}
	if t.FailPut != nil {
		if err := t.FailPut(key.PK + "/" + key.SK); err != nil {
			return nil, err
		}
	}
	if input.ConditionExpression == nil || *input.ConditionExpression == "" {
		return nil, errors.New("archive writes must protect newer source facts")
	}
	previous := t.items[key.PK+"/"+key.SK]
	var holds bool
	switch condition := *input.ConditionExpression; condition {
	case "attribute_not_exists(fetched_at) OR fetched_at <= :fetched_at":
		holds = previous == nil || fetchedAt(previous) <= stringValue(input.ExpressionAttributeValues[":fetched_at"])
	case "attribute_not_exists(pk)":
		holds = previous == nil
	case "#revision = :read_revision":
		holds = previous != nil && numberValue(previous[input.ExpressionAttributeNames["#revision"]]) == numberValue(input.ExpressionAttributeValues[":read_revision"])
	default:
		return nil, fmt.Errorf("archivetest does not evaluate condition %q", condition)
	}
	if !holds {
		return nil, &types.ConditionalCheckFailedException{}
	}
	t.items[key.PK+"/"+key.SK] = input.Item
	return &dynamodb.PutItemOutput{}, nil
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

// Query implements the archive's "pk = :pk" and
// "pk = :pk AND begins_with(sk, :prefix)" queries, paged by PageSize from
// ExclusiveStartKey.
func (t *Table) Query(_ context.Context, input *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	delete(t.items, key.PK+"/"+key.SK)
	return &dynamodb.DeleteItemOutput{}, nil
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
