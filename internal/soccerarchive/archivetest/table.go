// Package archivetest provides an in-memory DynamoDB table for exercising the
// durable Soccer archive without AWS.
package archivetest

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Table stores items by pk/sk. Puts must carry the archive's newest-fetch
// condition; an older fetched_at fails the condition as DynamoDB would.
type Table struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue
	// PutErr, when set, fails every put as an unavailable table would.
	PutErr error
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
	if t.PutErr != nil {
		return nil, t.PutErr
	}
	var key itemKey
	if err := attributevalue.UnmarshalMap(input.Item, &key); err != nil {
		return nil, err
	}
	if input.ConditionExpression == nil || *input.ConditionExpression == "" {
		return nil, errors.New("archive writes must protect newer source facts")
	}
	if previous := t.items[key.PK+"/"+key.SK]; previous != nil {
		if fetchedAt(previous) > fetchedAt(input.Item) {
			return nil, &types.ConditionalCheckFailedException{}
		}
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

// Query implements the archive's "pk = :pk AND begins_with(sk, :prefix)" query.
func (t *Table) Query(_ context.Context, input *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pk := stringValue(input.ExpressionAttributeValues[":pk"])
	prefix := stringValue(input.ExpressionAttributeValues[":prefix"])
	keys := make([]string, 0)
	for key := range t.items {
		if strings.HasPrefix(key, pk+"/"+prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	items := make([]map[string]types.AttributeValue, 0, len(keys))
	for _, key := range keys {
		items = append(items, t.items[key])
	}
	return &dynamodb.QueryOutput{Items: items}, nil
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

func stringValue(value types.AttributeValue) string {
	if s, ok := value.(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}
