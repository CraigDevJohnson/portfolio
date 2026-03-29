// Package google provides Google OAuth and Calendar API integration.
package google

import (
	"context"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ConnectionStore persists Google OAuth connection records.
type ConnectionStore interface {
	Delete(ctx context.Context, connectionID string) error
	Get(ctx context.Context, connectionID string) (*ConnectionRecord, error)
	Put(ctx context.Context, record *ConnectionRecord) error
}

// ConnectionRecord holds a persisted Google OAuth connection.
type ConnectionRecord struct {
	ConnectionID    string    `dynamodbav:"connection_id"`
	TokenCiphertext string    `dynamodbav:"token_ciphertext"`
	CalendarID      string    `dynamodbav:"calendar_id"`
	CalendarSummary string    `dynamodbav:"calendar_summary"`
	CreatedAt       time.Time `dynamodbav:"created_at"`
	UpdatedAt       time.Time `dynamodbav:"updated_at"`
}

// DynamoStore is a DynamoDB-backed ConnectionStore.
type DynamoStore struct {
	client    *dynamodb.Client
	tableName string
}

// NoopStore discards all operations — used when Google is not configured.
type NoopStore struct{}

// NewConnectionStore returns a DynamoStore if a table name is configured,
// otherwise a NoopStore.
func NewConnectionStore(ctx context.Context, tableName string) (ConnectionStore, error) {
	if strings.TrimSpace(tableName) == "" {
		return NoopStore{}, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &DynamoStore{
		client:    dynamodb.NewFromConfig(awsCfg),
		tableName: tableName,
	}, nil
}

func (NoopStore) Delete(_ context.Context, _ string) error                   { return nil }
func (NoopStore) Get(_ context.Context, _ string) (*ConnectionRecord, error) { return nil, nil }
func (NoopStore) Put(_ context.Context, _ *ConnectionRecord) error           { return nil }

func (s *DynamoStore) Delete(ctx context.Context, connectionID string) error {
	if strings.TrimSpace(connectionID) == "" {
		return nil
	}
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &s.tableName,
	})
	return err
}

func (s *DynamoStore) Get(ctx context.Context, connectionID string) (*ConnectionRecord, error) {
	if strings.TrimSpace(connectionID) == "" {
		return nil, nil
	}
	output, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &s.tableName,
	})
	if err != nil || len(output.Item) == 0 {
		return nil, err
	}
	var record ConnectionRecord
	if err := attributevalue.UnmarshalMap(output.Item, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *DynamoStore) Put(ctx context.Context, record *ConnectionRecord) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		Item:      item,
		TableName: &s.tableName,
	})
	return err
}
