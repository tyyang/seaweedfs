package engine

import (
	"context"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// TextToSQL converts natural language questions into SQL queries using Claude Fable 5.
// It uses the current database schema as context so the model can reference real table
// and column names.
type TextToSQL struct {
	client anthropic.Client
}

// NewTextToSQL creates a TextToSQL converter. The API key is read from
// ANTHROPIC_API_KEY (standard SDK default).
func NewTextToSQL() *TextToSQL {
	return &TextToSQL{
		client: anthropic.NewClient(option.WithMaxRetries(2)),
	}
}

// SchemaContext holds the schema information passed to the model.
type SchemaContext struct {
	Database string
	Tables   []TableSchema
}

// TableSchema describes a single table and its columns.
type TableSchema struct {
	Name    string
	Columns []ColumnInfo
}

// BuildSchemaContext gathers schema information from the catalog for the given
// database so the model knows what tables and columns are available.
func BuildSchemaContext(catalog *SchemaCatalog, database string) (*SchemaContext, error) {
	tables, err := catalog.ListTables(database)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	sc := &SchemaContext{Database: database}
	for _, tbl := range tables {
		info, err := catalog.GetTableInfo(database, tbl)
		if err != nil {
			// Skip tables we cannot introspect
			continue
		}
		sc.Tables = append(sc.Tables, TableSchema{
			Name:    tbl,
			Columns: info.Columns,
		})
	}
	return sc, nil
}

// schemaContextToText renders the schema context as a compact DDL-like string
// that fits cleanly into a prompt.
func schemaContextToText(sc *SchemaContext) string {
	if sc == nil {
		return "(no schema available)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Database: %s\n\n", sc.Database)
	for _, t := range sc.Tables {
		fmt.Fprintf(&sb, "Table: %s\n", t.Name)
		for _, c := range t.Columns {
			nullable := ""
			if c.Nullable {
				nullable = " NULL"
			}
			fmt.Fprintf(&sb, "  %s %s%s\n", c.Name, c.Type, nullable)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// GenerateSQL calls Claude Fable 5 to translate the natural-language question
// into a SQL statement for the SeaweedFS query engine.
//
// The SeaweedFS SQL engine supports PostgreSQL-compatible syntax with the
// following capabilities: SELECT, WHERE, LIMIT, OFFSET, aggregation functions
// (COUNT, SUM, AVG, MIN, MAX), SHOW DATABASES, SHOW TABLES, DESCRIBE, and
// CREATE TABLE. JOINs and sub-queries are not supported.
//
// Returns the generated SQL string and any error from the API call.
func (t *TextToSQL) GenerateSQL(ctx context.Context, question string, schema *SchemaContext) (string, error) {
	schemaText := schemaContextToText(schema)

	systemPrompt := `You are a SQL query generator for SeaweedFS, a distributed file and message-queue system.

The SeaweedFS SQL engine supports a PostgreSQL-compatible subset:
- SELECT with column projection, * wildcard, expressions, and aliases
- WHERE clauses: =, <, >, <=, >=, !=, <>, LIKE (% and _ wildcards), IN (value list), BETWEEN, IS NULL, IS NOT NULL
- Logical operators: AND, OR, NOT
- Aggregation: COUNT(*), COUNT(col), SUM(col), AVG(col), MIN(col), MAX(col)
- GROUP BY and HAVING
- ORDER BY col [ASC|DESC]
- LIMIT n [OFFSET m]
- Metadata: SHOW DATABASES, SHOW TABLES, SHOW TABLES FROM db, DESCRIBE table_name
- DDL: CREATE TABLE name (col1 TYPE, col2 TYPE, ...)

Constraints:
- No JOINs — each query targets a single table
- No sub-queries
- Timestamps are stored as BIGINT (nanoseconds since Unix epoch)
- Return ONLY the SQL statement — no explanation, no markdown, no code fences

The user is querying this schema:
` + schemaText

	message, err := t.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeFable5,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(question)),
		},
		// Use medium effort — SQL generation is a bounded task.
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortMedium,
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic API: %w", err)
	}

	if message.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("request declined by safety classifier (category: %s)", message.StopDetails.Category)
	}

	var result strings.Builder
	for _, block := range message.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			result.WriteString(tb.Text)
		}
	}

	sql := strings.TrimSpace(result.String())
	// Strip any accidental markdown code fences the model might emit.
	sql = strings.TrimPrefix(sql, "```sql")
	sql = strings.TrimPrefix(sql, "```")
	sql = strings.TrimSuffix(sql, "```")
	sql = strings.TrimSpace(sql)

	if sql == "" {
		return "", fmt.Errorf("model returned an empty response")
	}
	return sql, nil
}
