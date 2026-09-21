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

type CreaturePage struct {
	Creatures  []catalog.Creature
	NextCursor string
}

// maxBatchAttempts bounds the retry loop on unprocessed keys.
const maxBatchAttempts = 5

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

// GetCreature returns the creature id with its types and its movepool, in a single
// query over its partition. The boolean reports whether the creature exists.
func (s *Store) GetCreature(ctx context.Context, id string) (catalog.Creature, bool, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: creaturePK(id)},
		},
	})
	if err != nil {
		return catalog.Creature{}, false, fmt.Errorf("query creature %q: %w", id, err)
	}
	var (
		creature catalog.Creature
		moves    []catalog.LearnedMove
		found    bool
	)
	for _, raw := range out.Items {
		sk, ok := raw["SK"].(*types.AttributeValueMemberS)
		if !ok {
			return catalog.Creature{}, false, fmt.Errorf("creature %q: item without string SK", id)
		}

		if sk.Value == creatureEntity {
			var item creatureItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return catalog.Creature{}, false, fmt.Errorf("unmarshal creature %q: %w", id, err)
			}

			creature, err = item.creature()
			if err != nil {
				return catalog.Creature{}, false, err
			}
			found = true

			continue
		}

		// TYPE# items are ignored: the profile already carries the denormalised list.
		if _, isMove := idAfter(sk.Value, moveEntity); isMove {
			var item creatureMoveItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return catalog.Creature{}, false, fmt.Errorf("unmarshal movepool of %q: %w", id, err)
			}

			move, err := item.learnedMove()
			if err != nil {
				return catalog.Creature{}, false, err
			}

			moves = append(moves, move)
		}
	}
	if !found {
		return catalog.Creature{}, false, nil
	}
	creature.Moves = moves

	return creature, true, nil
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// queryCreaturePage runs one page of a query on the inverted index for the given sort
// key, and returns the raw items with the token of the next page.
func (s *Store) queryCreaturePage(ctx context.Context, sk string, limit int32, token string) ([]map[string]types.AttributeValue, string, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = defaultPageSize
	}

	start, err := decodeCursor(token)
	if err != nil {
		return nil, "", err
	}

	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		IndexName:              aws.String(indexInverted),
		KeyConditionExpression: aws.String("SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: sk},
		},
		Limit:             aws.Int32(limit),
		ExclusiveStartKey: start,
	})
	if err != nil {
		return nil, "", fmt.Errorf("query index on %q: %w", sk, err)
	}

	next, err := encodeCursor(out.LastEvaluatedKey)
	if err != nil {
		return nil, "", err
	}

	return out.Items, next, nil
}

// ListCreatures returns one page of creature profiles, movepools excluded.
// An empty NextCursor means the last page was reached.
func (s *Store) ListCreatures(ctx context.Context, limit int32, token string) (CreaturePage, error) {
	items, next, err := s.queryCreaturePage(ctx, creatureEntity, limit, token)
	if err != nil {
		return CreaturePage{}, err
	}

	creatures := make([]catalog.Creature, 0, len(items))
	for _, raw := range items {
		var item creatureItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return CreaturePage{}, fmt.Errorf("unmarshal creature: %w", err)
		}

		c, err := item.creature()
		if err != nil {
			return CreaturePage{}, err
		}

		creatures = append(creatures, c)
	}

	return CreaturePage{Creatures: creatures, NextCursor: next}, nil
}

// batchGetCreatures reads the profiles behind keys and returns them in order of keys.
func (s *Store) batchGetCreatures(ctx context.Context, keys []map[string]types.AttributeValue) ([]catalog.Creature, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	byID := make(map[string]catalog.Creature, len(keys))
	pending := keys

	for attempt := 0; len(pending) > 0; attempt++ {
		if attempt == maxBatchAttempts {
			return nil, fmt.Errorf("batch get creatures: %d keys unprocessed after %d attempts", len(pending), attempt)
		}

		out, err := s.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				s.table: {Keys: pending},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("batch get creatures: %w", err)
		}

		for _, raw := range out.Responses[s.table] {
			var item creatureItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return nil, fmt.Errorf("unmarshal creature: %w", err)
			}

			c, err := item.creature()
			if err != nil {
				return nil, err
			}
			byID[c.ID] = c
		}

		pending = out.UnprocessedKeys[s.table].Keys
	}

	creatures := make([]catalog.Creature, 0, len(keys))
	for _, key := range keys {
		pk, ok := key["PK"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}
		id, ok := idAfter(pk.Value, creatureEntity)
		if !ok {
			continue
		}
		if c, ok := byID[id]; ok {
			creatures = append(creatures, c)
		}
	}

	return creatures, nil
}

// ListCreaturesByType returns one page of the creatures having the given type.
func (s *Store) ListCreaturesByType(ctx context.Context, typeID string, limit int32, token string) (CreaturePage, error) {
	if !catalog.IsSlug(typeID) {
		return CreaturePage{}, fmt.Errorf("%w: type %q must be a slug", catalog.ErrInvalid, typeID)
	}

	items, next, err := s.queryCreaturePage(ctx, creatureTypeSK(typeID), limit, token)
	if err != nil {
		return CreaturePage{}, err
	}

	// The index yields link items, which carry keys only: the profiles are read after.
	keys := make([]map[string]types.AttributeValue, 0, len(items))
	for _, raw := range items {
		pk, ok := raw["PK"].(*types.AttributeValueMemberS)
		if !ok {
			return CreaturePage{}, fmt.Errorf("type %q: link item without a string PK", typeID)
		}

		keys = append(keys, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk.Value},
			"SK": &types.AttributeValueMemberS{Value: creatureEntity},
		})
	}

	creatures, err := s.batchGetCreatures(ctx, keys)
	if err != nil {
		return CreaturePage{}, err
	}

	return CreaturePage{Creatures: creatures, NextCursor: next}, nil
}
