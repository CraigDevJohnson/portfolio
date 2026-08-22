package soccer

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"portfolio/types"
)

// SoccerStore writes imported soccer session baselines.
type SoccerStore interface {
	Put(ctx context.Context, record *SoccerSessionRecord) error
}

// SoccerSessionRecord is the DynamoDB record for a soccer session baseline.
type SoccerSessionRecord struct {
	SessionID   string    `dynamodbav:"session_id"`
	UserName    string    `dynamodbav:"user_name"`
	PlayersJSON string    `dynamodbav:"players_json"`
	StartedAt   time.Time `dynamodbav:"started_at"`
	ExpiresAt   time.Time `dynamodbav:"expires_at"`
	TTL         int64     `dynamodbav:"ttl"`
}

// DynamoSoccerStore implements SoccerStore using DynamoDB.
type DynamoSoccerStore struct {
	client    *dynamodb.Client
	tableName string
}

// NoopSoccerStore is a no-op SoccerStore used when soccer session persistence is disabled.
type NoopSoccerStore struct{}

// NewSoccerStore returns a DynamoSoccerStore when a table name is configured,
// otherwise a NoopSoccerStore.
func NewSoccerStore(ctx context.Context, tableName string) (SoccerStore, error) {
	if strings.TrimSpace(tableName) == "" {
		return NoopSoccerStore{}, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &DynamoSoccerStore{
		client:    dynamodb.NewFromConfig(awsCfg),
		tableName: tableName,
	}, nil
}

func (NoopSoccerStore) Put(_ context.Context, _ *SoccerSessionRecord) error { return nil }

func (s *DynamoSoccerStore) Put(ctx context.Context, record *SoccerSessionRecord) error {
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

func marshalPlayersJSON(players []types.LPSPlayer) (string, error) {
	if len(players) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(players)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
