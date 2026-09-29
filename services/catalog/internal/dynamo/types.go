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

// typeItem is a type and the row of the chart it attacks with:
// PK = TYPE#<id>, SK = TYPE.
type typeItem struct {
	PK            string             `dynamodbav:"PK"`
	SK            string             `dynamodbav:"SK"`
	Name          string             `dynamodbav:"name"`
	Effectiveness map[string]float64 `dynamodbav:"effectiveness,omitempty"`
}

// newTypeItem builds the item stored for t.
func newTypeItem(t catalog.Type) typeItem {
	effectiveness := t.Effectiveness
	if len(effectiveness) == 0 {
		effectiveness = nil
	}

	return typeItem{
		PK:            typePK(t.ID),
		SK:            typeEntity,
		Name:          t.Name,
		Effectiveness: effectiveness,
	}
}

// elementalType rebuilds the domain value from the item.
func (i typeItem) elementalType() (catalog.Type, error) {
	id, ok := idAfter(i.PK, typeEntity)
	if !ok {
		return catalog.Type{}, fmt.Errorf("malformed type item: PK %q", i.PK)
	}

	return catalog.Type{
		ID:            id,
		Name:          i.Name,
		Effectiveness: i.Effectiveness,
	}, nil
}

// PutType stores t, replacing any type already stored under the same identifier.
func (s *Store) PutType(ctx context.Context, t catalog.Type) error {
	if err := t.Validate(); err != nil {
		return err
	}

	item, err := attributevalue.MarshalMap(newTypeItem(t))
	if err != nil {
		return fmt.Errorf("marshal type %q: %w", t.ID, err)
	}

	if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item:      item,
	}); err != nil {
		return fmt.Errorf("put type %q: %w", t.ID, err)
	}

	return nil
}

// GetType returns the type stored under id.
// The boolean is false when no type has that identifier, which is not an error.
func (s *Store) GetType(ctx context.Context, id string) (catalog.Type, bool, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: typePK(id)},
			"SK": &types.AttributeValueMemberS{Value: typeEntity},
		},
	})
	if err != nil {
		return catalog.Type{}, false, fmt.Errorf("get type %q: %w", id, err)
	}
	if out.Item == nil {
		return catalog.Type{}, false, nil
	}

	var item typeItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return catalog.Type{}, false, fmt.Errorf("unmarshal type %q: %w", id, err)
	}

	t, err := item.elementalType()
	if err != nil {
		return catalog.Type{}, false, err
	}

	return t, true, nil
}

// ListTypes returns one page of types, ordered by identifier.
// An empty NextCursor means the last page was reached.
func (s *Store) ListTypes(ctx context.Context, limit int32, token string) (catalog.TypePage, error) {
	items, next, err := s.queryIndexPage(ctx, typeEntity, limit, token)
	if err != nil {
		return catalog.TypePage{}, err
	}

	elementalTypes := make([]catalog.Type, 0, len(items))
	for _, raw := range items {
		var item typeItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return catalog.TypePage{}, fmt.Errorf("unmarshal type: %w", err)
		}

		t, err := item.elementalType()
		if err != nil {
			return catalog.TypePage{}, err
		}

		elementalTypes = append(elementalTypes, t)
	}

	return catalog.TypePage{Types: elementalTypes, NextCursor: next}, nil
}
