package dynamo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// moveItem is a move: PK = MOVE#<id>, SK = MOVE.
type moveItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	Name        string `dynamodbav:"name"`
	Type        string `dynamodbav:"type"`
	DamageClass string `dynamodbav:"damageClass"`
	Power       int    `dynamodbav:"power"`
	Accuracy    int    `dynamodbav:"accuracy"`
	PP          int    `dynamodbav:"pp"`
}

// newMoveItem builds the item stored for m.
func newMoveItem(m catalog.Move) moveItem {
	return moveItem{
		PK:          movePK(m.ID),
		SK:          moveEntity,
		Name:        m.Name,
		Type:        m.Type,
		DamageClass: string(m.DamageClass),
		Power:       m.Power,
		Accuracy:    m.Accuracy,
		PP:          m.PP,
	}
}

// move rebuilds the domain value from the item.
func (i moveItem) move() (catalog.Move, error) {
	id, ok := idAfter(i.PK, moveEntity)
	if !ok {
		return catalog.Move{}, fmt.Errorf("malformed move item: PK %q", i.PK)
	}

	return catalog.Move{
		ID:          id,
		Name:        i.Name,
		Type:        i.Type,
		DamageClass: catalog.DamageClass(i.DamageClass),
		Power:       i.Power,
		Accuracy:    i.Accuracy,
		PP:          i.PP,
	}, nil
}

// PutMove stores m, replacing any move already stored under the same identifier.
func (s *Store) PutMove(ctx context.Context, m catalog.Move) error {
	if err := m.Validate(); err != nil {
		return err
	}

	item, err := attributevalue.MarshalMap(newMoveItem(m))
	if err != nil {
		return fmt.Errorf("marshal move %q: %w", m.ID, err)
	}

	if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item:      item,
	}); err != nil {
		return fmt.Errorf("put move %q: %w", m.ID, err)
	}

	return nil
}

// GetMove returns the move stored under id.
// The boolean is false when no move has that identifier, which is not an error.
func (s *Store) GetMove(ctx context.Context, id string) (catalog.Move, bool, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: movePK(id)},
			"SK": &types.AttributeValueMemberS{Value: moveEntity},
		},
	})
	if err != nil {
		return catalog.Move{}, false, fmt.Errorf("get move %q: %w", id, err)
	}
	if out.Item == nil {
		return catalog.Move{}, false, nil
	}

	var item moveItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return catalog.Move{}, false, fmt.Errorf("unmarshal move %q: %w", id, err)
	}

	m, err := item.move()
	if err != nil {
		return catalog.Move{}, false, err
	}

	return m, true, nil
}

// ListMoves returns one page of moves, ordered by identifier.
// An empty NextCursor means the last page was reached.
func (s *Store) ListMoves(ctx context.Context, limit int32, token string) (catalog.MovePage, error) {
	items, next, err := s.queryIndexPage(ctx, moveEntity, limit, token)
	if err != nil {
		return catalog.MovePage{}, err
	}

	moves := make([]catalog.Move, 0, len(items))
	for _, raw := range items {
		var item moveItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return catalog.MovePage{}, fmt.Errorf("unmarshal move: %w", err)
		}

		m, err := item.move()
		if err != nil {
			return catalog.MovePage{}, err
		}

		moves = append(moves, m)
	}

	return catalog.MovePage{Moves: moves, NextCursor: next}, nil
}
