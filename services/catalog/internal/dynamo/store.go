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

// Store reads and writes the catalog in a single DynamoDB table.
type Store struct {
	client *dynamodb.Client
	table  string
}

// New returns a Store backed by client, operating on table.
func New(client *dynamodb.Client, table string) *Store {
	return &Store{client: client, table: table}
}

// removed returns the identifiers present in previous but absent from current.
func removed(previous, current []string) []string {
	kept := make(map[string]bool, len(current))
	for _, id := range current {
		kept[id] = true
	}

	var gone []string
	for _, id := range previous {
		if !kept[id] {
			gone = append(gone, id)
		}
	}
	return gone
}

// creatureTypes returns the types currently stored for id.
// A creature that does not exist yet has none, which is not an error.
func (s *Store) creatureTypes(ctx context.Context, id string) ([]string, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: creaturePK(id)},
			"SK": &types.AttributeValueMemberS{Value: creatureEntity},
		},
		ProjectionExpression:     aws.String("#types"),
		ExpressionAttributeNames: map[string]string{"#types": "types"},
	})
	if err != nil {
		return nil, fmt.Errorf("read types of %q: %w", id, err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var item creatureItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshal types of %q: %w", id, err)
	}

	return item.Types, nil
}

// PutCreature writes the profile and the type links of c in a single transaction,
// so the denormalised copies cannot diverge (ADR-0010). Types the creature no
// longer has are deleted in the same transaction.
func (s *Store) PutCreature(ctx context.Context, c catalog.Creature) error {
	if err := c.Validate(); err != nil {
		return err
	}

	previous, err := s.creatureTypes(ctx, c.ID)
	if err != nil {
		return err
	}

	profile, err := attributevalue.MarshalMap(newCreatureItem(c))
	if err != nil {
		return fmt.Errorf("marshal creature %q: %w", c.ID, err)
	}

	writes := []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(s.table), Item: profile}},
	}

	for _, link := range newCreatureTypeItems(c) {
		item, err := attributevalue.MarshalMap(link)
		if err != nil {
			return fmt.Errorf("marshal type link of %q: %w", c.ID, err)
		}
		writes = append(writes, types.TransactWriteItem{
			Put: &types.Put{TableName: aws.String(s.table), Item: item},
		})
	}

	for _, typeID := range removed(previous, c.Types) {
		writes = append(writes, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: creaturePK(c.ID)},
					"SK": &types.AttributeValueMemberS{Value: creatureTypeSK(typeID)},
				},
			},
		})
	}

	if _, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: writes,
	}); err != nil {
		return fmt.Errorf("put creature %q: %w", c.ID, err)
	}

	return nil
}
