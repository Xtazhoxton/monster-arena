package dynamo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// cursor is the opaque pagination token handed to clients: the last key of a page.
// Warning: the token is readable and forgeable by clients. If this table ever holds
// private data, sign the cursor instead of trusting it.
type cursor struct {
	PK string `dynamodbav:"PK" json:"pk"`
	SK string `dynamodbav:"SK" json:"sk"`
}

// encodeCursor turns the last evaluated key of a page into an opaque token.
// An empty key means there is no next page, and yields an empty token.
func encodeCursor(key map[string]types.AttributeValue) (string, error) {
	if len(key) == 0 {
		return "", nil
	}

	var c cursor
	if err := attributevalue.UnmarshalMap(key, &c); err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}

	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeCursor turns a client token back into an exclusive start key.
// An empty token means "start from the beginning".
func decodeCursor(token string) (map[string]types.AttributeValue, error) {
	if token == "" {
		return nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor is not valid base64", catalog.ErrInvalid)
	}

	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%w: cursor is not valid JSON", catalog.ErrInvalid)
	}

	if c.PK == "" || c.SK == "" {
		return nil, fmt.Errorf("%w: cursor is missing a key", catalog.ErrInvalid)
	}

	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: c.PK},
		"SK": &types.AttributeValueMemberS{Value: c.SK},
	}, nil
}
