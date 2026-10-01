package google

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// fakeConnectionTable answers DynamoDB PutItem requests for one stored
// connection, applying the condition PutIfUnchanged sends: the connection
// must exist and still carry the update time the caller read.
type fakeConnectionTable struct {
	t         *testing.T
	mu        sync.Mutex
	updatedAt string
	puts      int
}

func (table *fakeConnectionTable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	table.mu.Lock()
	defer table.mu.Unlock()
	if r.Header.Get("X-Amz-Target") != "DynamoDB_20120810.PutItem" {
		table.t.Errorf("unexpected DynamoDB request %q", r.Header.Get("X-Amz-Target"))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var input struct {
		ConditionExpression       string
		ExpressionAttributeValues map[string]map[string]any
		Item                      map[string]map[string]any
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		table.t.Fatalf("PutItem body: %v", err)
	}
	if input.ConditionExpression != "attribute_exists(connection_id) AND updated_at = :read_updated_at" {
		table.t.Errorf("PutItem condition = %q", input.ConditionExpression)
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	if table.updatedAt == "" || input.ExpressionAttributeValues[":read_updated_at"]["S"] != table.updatedAt {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"The conditional request failed"}`))
		return
	}
	table.updatedAt, _ = input.Item["updated_at"]["S"].(string)
	table.puts++
	_, _ = w.Write([]byte(`{}`))
}

func (table *fakeConnectionTable) saved() int {
	table.mu.Lock()
	defer table.mu.Unlock()
	return table.puts
}

func (table *fakeConnectionTable) remove() {
	table.mu.Lock()
	defer table.mu.Unlock()
	table.updatedAt = ""
}

func TestDynamoStorePutIfUnchangedSavesOnlyOverTheCopyThatWasRead(t *testing.T) {
	read := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	table := &fakeConnectionTable{t: t, updatedAt: read.Format(time.RFC3339Nano)}
	server := httptest.NewServer(table)
	t.Cleanup(server.Close)
	store := &DynamoStore{
		client: dynamodb.New(dynamodb.Options{
			BaseEndpoint: aws.String(server.URL),
			Region:       "us-west-2",
			Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
			}),
			RetryMaxAttempts: 1,
		}),
		tableName: "connections",
	}
	record := &ConnectionRecord{ConnectionID: "connection-1", CalendarID: "team", UpdatedAt: read.Add(time.Minute)}

	if err := store.PutIfUnchanged(context.Background(), record, read); err != nil || table.saved() != 1 {
		t.Fatalf("saving over the copy that was read = %v after %d puts; want it saved", err, table.saved())
	}
	// The table now holds the save above, so the earlier read is stale.
	if err := store.PutIfUnchanged(context.Background(), record, read); !errors.Is(err, ErrConnectionChanged) || table.saved() != 1 {
		t.Fatalf("saving over a copy changed since it was read = %v after %d puts; want ErrConnectionChanged and nothing saved", err, table.saved())
	}
	table.remove()
	if err := store.PutIfUnchanged(context.Background(), record, record.UpdatedAt); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("saving a removed connection = %v; want ErrConnectionChanged", err)
	}
}
