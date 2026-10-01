package soccerarchive

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ErrAdmissionFull means a new team cannot join daily refresh because the
// reviewed enrollment capacity for its source is used.
var ErrAdmissionFull = errors.New("soccer history admission full")

// AdmissionRejectedLog is the structured log message a caller records, once
// for each refused team, whenever a new team is refused at capacity. The
// admission alarm counts it.
const AdmissionRejectedLog = "soccer_history_admission_rejected"

// AdmissionError names the new teams refused because the reviewed admission
// capacity for their enrollment source is used. It wraps ErrAdmissionFull.
// The store does not log the refusal; the caller that reports it to the
// visitor records AdmissionRejectedLog with its request context.
type AdmissionError struct {
	Source  string
	Limit   int
	TeamIDs []int
}

func (e *AdmissionError) Error() string {
	ids := make([]string, 0, len(e.TeamIDs))
	for _, teamID := range e.TeamIDs {
		ids = append(ids, strconv.Itoa(teamID))
	}
	return fmt.Sprintf("%v: %s team %s over admission limit %d", ErrAdmissionFull, e.Source, strings.Join(ids, ", "), e.Limit)
}

func (e *AdmissionError) Unwrap() error { return ErrAdmissionFull }

const (
	// manualEnrollment marks a team a visitor entered by Team ID.
	manualEnrollment = "manual"

	// The admission counter counts every enrolled team, whatever its source.
	capacityPK = "CAPACITY#SOCCER_HISTORY"
	capacitySK = "ENROLLED"
)

// admissionLimit is how many enrolled teams a new team from source may join.
// Anonymous manual IDs stop short of the slots reserved for teams an
// authenticated player import found.
func (s *DynamoStore) admissionLimit(source string) int {
	if source == playerEnrollment {
		return s.limits.MaxEnrolledTeams
	}
	return s.limits.MaxEnrolledTeams - s.limits.ReservedPlayerSlots
}

// enrolledCount reads the admission counter and the revision it was read at.
func (s *DynamoStore) enrolledCount(ctx context.Context) (count int, counter *archiveItem, err error) {
	counter, err = s.get(ctx, capacityPK, capacitySK)
	if err != nil || counter == nil {
		return 0, counter, err
	}
	return counter.EnrolledCount, counter, nil
}

// checkAdmission refuses, before anything is written, new teams that would
// not fit the capacity left for source. Teams already enrolled need no slot.
// It is a read, not a reservation: a concurrent enrollment can still take
// the last slot before the caller's own enrollment, which enrollNew then
// refuses.
func (s *DynamoStore) checkAdmission(ctx context.Context, source string, teamIDs ...int) error {
	newTeams := make([]int, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		previous, err := s.get(ctx, teamKey(teamID), "META")
		if err != nil {
			return err
		}
		if previous == nil {
			newTeams = append(newTeams, teamID)
		}
	}
	if len(newTeams) == 0 {
		return nil
	}
	count, _, err := s.enrolledCount(ctx)
	if err != nil {
		return err
	}
	if limit := s.admissionLimit(source); count+len(newTeams) > limit {
		return admissionRejected(source, limit, newTeams...)
	}
	return nil
}

// enrollNew writes a new team's enrollment record and takes one admission
// slot in a single transaction, so capacity can never be exceeded and no
// slot is counted without its team. It reports false when another writer
// created the team first, even if that writer took the last slot; the caller
// then merges into that record instead. A transaction DynamoDB cancels over
// a concurrent one on the same items is retried.
func (s *DynamoStore) enrollNew(ctx context.Context, record *archiveItem, source string) (bool, error) {
	teamItem, err := attributevalue.MarshalMap(record)
	if err != nil {
		return false, err
	}
	for range maxRecordWriteAttempts {
		count, counter, err := s.enrolledCount(ctx)
		if err != nil {
			return false, err
		}
		limit := s.admissionLimit(source)
		if count >= limit {
			// The slot that filled capacity may be this very team's, taken by
			// another writer after the caller read the team as new.
			existing, err := s.get(ctx, record.PK, record.SK)
			if err != nil {
				return false, err
			}
			if existing != nil {
				return false, nil
			}
			return false, admissionRejected(source, limit, record.TeamID)
		}
		next := archiveItem{PK: capacityPK, SK: capacitySK, Kind: "capacity", EnrolledCount: count + 1, Revision: 1}
		counterPut := &types.Put{TableName: aws.String(s.tableName), ConditionExpression: aws.String("attribute_not_exists(pk)")}
		if counter != nil {
			next.Revision = counter.Revision + 1
			counterPut.ConditionExpression = aws.String("#revision = :read_revision")
			counterPut.ExpressionAttributeNames = map[string]string{"#revision": "revision"}
			counterPut.ExpressionAttributeValues = map[string]types.AttributeValue{
				":read_revision": &types.AttributeValueMemberN{Value: strconv.Itoa(counter.Revision)},
			}
		}
		if counterPut.Item, err = attributevalue.MarshalMap(&next); err != nil {
			return false, err
		}
		_, err = s.api.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.tableName), Item: teamItem, ConditionExpression: aws.String("attribute_not_exists(pk)")}},
			{Put: counterPut},
		}})
		var canceled *types.TransactionCanceledException
		switch {
		case err == nil:
			return true, nil
		case !errors.As(err, &canceled):
			return false, fmt.Errorf("enroll team %d: %w", record.TeamID, err)
		case cancellationCode(canceled, 0) == "ConditionalCheckFailed":
			return false, nil
		case cancellationCode(canceled, 1) != "ConditionalCheckFailed" && !transactionConflict(canceled):
			return false, fmt.Errorf("enroll team %d: %w", record.TeamID, err)
		}
		// Another team took a slot first, or DynamoDB canceled this
		// transaction over a concurrent one; re-read the counter and retry.
	}
	return false, fmt.Errorf("enroll team %d: capacity changed by concurrent enrollments %d times", record.TeamID, maxRecordWriteAttempts)
}

// cancellationCode is the reason DynamoDB gave for canceling the
// transaction's item at index, or "" when it gave none.
func cancellationCode(canceled *types.TransactionCanceledException, index int) string {
	if index >= len(canceled.CancellationReasons) {
		return ""
	}
	return aws.ToString(canceled.CancellationReasons[index].Code)
}

// transactionConflict reports a transaction canceled because another
// transaction was writing the team or counter item at the same time.
func transactionConflict(canceled *types.TransactionCanceledException) bool {
	return cancellationCode(canceled, 0) == "TransactionConflict" || cancellationCode(canceled, 1) == "TransactionConflict"
}

func admissionRejected(source string, limit int, teamIDs ...int) error {
	return &AdmissionError{Source: source, Limit: limit, TeamIDs: teamIDs}
}
