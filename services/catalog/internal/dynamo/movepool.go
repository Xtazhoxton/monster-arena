package dynamo

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// maxBatchWrite is the ceiling of a single BatchWriteItem call.
const maxBatchWrite = 25

// movepoolOf returns the move identifiers stored for the creature id, and whether
// that creature exists. The read is strongly consistent: a move written a moment ago
// must be seen, or a replacement would never delete it.
func (s *Store) movepoolOf(ctx context.Context, id string) ([]string, bool, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: creaturePK(id)},
		},
		ProjectionExpression: aws.String("SK"),
		ConsistentRead:       aws.Bool(true),
	})
	if err != nil {
		return nil, false, fmt.Errorf("read movepool of %q: %w", id, err)
	}
	var (
		moveIDs []string
		found   bool
	)
	for _, raw := range out.Items {
		sk, ok := raw["SK"].(*types.AttributeValueMemberS)
		if !ok {
			return nil, false, fmt.Errorf("creature %q: item without string SK", id)
		}

		if sk.Value == creatureEntity {
			found = true
			continue
		}
		if moveID, isMove := idAfter(sk.Value, moveEntity); isMove {
			moveIDs = append(moveIDs, moveID)
		}
	}

	return moveIDs, found, nil
}

// batchWrite sends requests in a single BatchWriteItem, resending what DynamoDB left
// unprocessed.
func (s *Store) batchWrite(ctx context.Context, requests []types.WriteRequest) error {
	pending := requests
	for attempt := 0; len(pending) > 0; attempt++ {
		if attempt == maxBatchAttempts {
			return fmt.Errorf("%d writes unprocessed after %d attempts", len(pending), attempt)
		}
		out, err := s.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.table: pending},
		})
		if err != nil {
			return err
		}
		pending = out.UnprocessedItems[s.table]
	}
	return nil
}

// PutMovepool replaces the movepool of the creature id with p: moves absent from p
// are deleted. The boolean is false when no such creature exists; nothing is written
// then. The replacement is not atomic — a movepool exceeds the 100 items of a
// transaction — but it is idempotent: after a failure, the same call converges.
func (s *Store) PutMovepool(ctx context.Context, id string, p catalog.Movepool) (bool, error) {
	if !catalog.IsSlug(id) {
		return false, fmt.Errorf("%w: id %q must be a slug", catalog.ErrInvalid, id)
	}
	if err := p.Validate(); err != nil {
		return false, err
	}
	previous, found, err := s.movepoolOf(ctx, id)
	if err != nil {
		return false, err
	}

	if !found {
		return false, nil
	}

	requests := make([]types.WriteRequest, 0, len(p.Moves))
	current := make([]string, 0, len(p.Moves))

	for _, m := range p.Moves {
		item, err := attributevalue.MarshalMap(newCreatureMoveItem(id, m))
		if err != nil {
			return false, fmt.Errorf("marshal move %q of %q: %w", m.MoveID, id, err)
		}

		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})
		current = append(current, m.MoveID)
	}

	for _, moveID := range removed(previous, current) {
		requests = append(requests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: creaturePK(id)},
					"SK": &types.AttributeValueMemberS{Value: creatureMoveSK(moveID)},
				},
			},
		})
	}
	for batch := range slices.Chunk(requests, maxBatchWrite) {
		if err := s.batchWrite(ctx, batch); err != nil {
			return false, fmt.Errorf("put movepool of %q: %w", id, err)
		}
	}
	return true, nil
}
