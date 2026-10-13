package api

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// Спеки — единственный источник HTTP-контрактов (06-api-contracts.md §6, §9.1).
var (
	ingestSpecPath   = filepath.Join("..", "..", "..", "contracts", "openapi", "ingest.yaml")
	apiSpecPath      = filepath.Join("..", "..", "..", "contracts", "openapi", "api.yaml")
	requestsHTTPPath = filepath.Join("..", "..", "..", "contracts", "http", "requests.http")
)

const adminPrefix = "/admin/v1"

var wantIngestOperationIDs = []string{"preflightEvents", "ingestEvents", "ingestHealth", "ingestReady"}

var wantAPIOperationIDs = []string{
	// Health и Read API /v1.
	"apiHealth", "apiReady",
	"listApps", "getApp", "getPortfolio",
	"getAudienceSummary", "getAudienceTimeseries", "getRollingActive", "getAudienceBreakdown",
	"getRetention", "getRealtime",
	"searchProfiles", "getProfile",
	"getIntegrationHealth",
	"createDeletionRequest", "getDeletionRequest",
	// Admin API /admin/v1.
	"getAuthState", "beginLogin", "finishLogin", "beginSetup", "finishSetup", "logout",
	"listPasskeys", "beginPasskeyRegistration", "finishPasskeyRegistration", "renamePasskey", "deletePasskey",
	"createApp", "updateApp", "deleteApp", "archiveApp", "unarchiveApp",
	"listWriteKeys", "createWriteKey", "revokeWriteKey",
	"listApiKeys", "createApiKey", "revokeApiKey",
	"getSettings", "updateSettings", "sendTelegramTest", "getSystemStatus", "listDeletionRequests",
}

type operation struct {
	ID       string
	Method   string
	Path     string
	Internal bool
}

func loadSpec(t *testing.T, path string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, err, "load %s", path)
	return doc
}

func loadSpecData(t *testing.T, data string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(data))
	require.NoError(t, err)
	return doc
}

// operations возвращает операции спеки в детерминированном порядке (путь, метод).
func operations(doc *openapi3.T) []operation {
	var ops []operation
	for _, path := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Value(path)
		for method, op := range item.Operations() {
			internal, _ := op.Extensions["x-internal"].(bool)
			ops = append(ops, operation{ID: op.OperationID, Method: method, Path: path, Internal: internal})
		}
	}
	slices.SortFunc(ops, func(a, b operation) int {
		return strings.Compare(a.Path+" "+a.Method, b.Path+" "+b.Method)
	})
	return ops
}

func operationIDs(ops []operation) []string {
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, op.ID)
	}
	return ids
}

// duplicateIDs возвращает пустые и повторяющиеся operationId.
func duplicateIDs(ops []operation) []string {
	seen := make(map[string]bool, len(ops))
	var dups []string
	for _, op := range ops {
		if op.ID == "" || seen[op.ID] {
			dups = append(dups, op.Method+" "+op.Path+" ("+op.ID+")")
		}
		seen[op.ID] = true
	}
	return dups
}

// internalViolations возвращает операции, у которых x-internal не соответствует префиксу пути:
// весь /admin/v1 скрыт из документации, а публичные операции — нет.
func internalViolations(ops []operation) []string {
	var bad []string
	for _, op := range ops {
		if strings.HasPrefix(op.Path, adminPrefix) != op.Internal {
			bad = append(bad, op.Method+" "+op.Path)
		}
	}
	return bad
}

var requestNameRe = regexp.MustCompile(`(?m)^# @name (\w+)\s*$`)

// missingRequests возвращает публичные операции без запроса `# @name <operationId>` в requests.http.
func missingRequests(ops []operation, requestsHTTP string) []string {
	var named []string
	for _, m := range requestNameRe.FindAllStringSubmatch(requestsHTTP, -1) {
		named = append(named, m[1])
	}
	var missing []string
	for _, op := range ops {
		if !op.Internal && !slices.Contains(named, op.ID) {
			missing = append(missing, op.ID)
		}
	}
	return missing
}

func TestSpecs_LoadAndValidate(t *testing.T) {
	for _, path := range []string{ingestSpecPath, apiSpecPath} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc := loadSpec(t, path)
			require.Equal(t, "3.0.3", doc.OpenAPI)
			require.NoError(t, doc.Validate(t.Context()))
		})
	}
}

func TestSpecs_OperationIDs(t *testing.T) {
	ops := operations(loadSpec(t, apiSpecPath))

	require.Len(t, ops, 43)
	require.Empty(t, duplicateIDs(ops), "operationId должны быть непустыми и уникальными")
	require.ElementsMatch(t, wantAPIOperationIDs, operationIDs(ops))
	require.Empty(t, internalViolations(ops), "x-internal: true ровно у путей %s", adminPrefix)
}

func TestSpecs_IngestOperations(t *testing.T) {
	doc := loadSpec(t, ingestSpecPath)
	ops := operations(doc)

	require.Empty(t, duplicateIDs(ops))
	require.ElementsMatch(t, wantIngestOperationIDs, operationIDs(ops))
	require.Empty(t, internalViolations(ops))

	batch := doc.Components.Schemas["EventBatch"]
	require.NotNil(t, batch, "схема EventBatch")
	require.Subset(t, batch.Value.Required, []string{"sent_at", "events"})

	events := batch.Value.Properties["events"]
	require.NotNil(t, events, "EventBatch.events")
	require.Equal(t, uint64(1), events.Value.MinItems)
	require.NotNil(t, events.Value.MaxItems)
	require.Equal(t, uint64(500), *events.Value.MaxItems)
}

func TestRequestsHTTP_CoversPublicOperations(t *testing.T) {
	data, err := os.ReadFile(requestsHTTPPath)
	require.NoError(t, err)

	for _, path := range []string{ingestSpecPath, apiSpecPath} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			ops := operations(loadSpec(t, path))
			require.Empty(t, missingRequests(ops, string(data)), "нет запроса в requests.http")
		})
	}
}

const brokenSpec = `
openapi: 3.0.3
info: { title: broken, version: 1.0.0 }
paths:
  /v1/apps:
    get:
      operationId: listApps
      responses:
        '200': { description: ok }
  /v1/apps/{app_id}:
    get:
      operationId: listApps
      responses:
        '200': { description: ok }
  /admin/v1/apps:
    get:
      operationId: adminListApps
      responses:
        '200': { description: ok }
  /admin/v1/settings:
    get:
      operationId: getSettings
      x-internal: true
      responses:
        '200': { description: ok }
    patch:
      responses:
        '200': { description: ok }
`

func TestSpecs_DetectsViolations(t *testing.T) {
	ops := operations(loadSpecData(t, brokenSpec))

	t.Run("duplicate and empty operationId", func(t *testing.T) {
		require.Equal(t, []string{
			"PATCH /admin/v1/settings ()",
			"GET /v1/apps/{app_id} (listApps)",
		}, duplicateIDs(ops))
	})

	t.Run("admin path without x-internal", func(t *testing.T) {
		require.Equal(t, []string{"GET /admin/v1/apps", "PATCH /admin/v1/settings"}, internalViolations(ops))
	})

	t.Run("public path marked x-internal", func(t *testing.T) {
		pub := []operation{{ID: "listApps", Method: "GET", Path: "/v1/apps", Internal: true}}
		require.Equal(t, []string{"GET /v1/apps"}, internalViolations(pub))
	})

	t.Run("missing request in requests.http", func(t *testing.T) {
		ops := []operation{
			{ID: "listApps", Method: "GET", Path: "/v1/apps"},
			{ID: "getApp", Method: "GET", Path: "/v1/apps/{app_id}"},
			{ID: "getSettings", Method: "GET", Path: "/admin/v1/settings", Internal: true},
		}
		requests := "# @name listApps\nGET {{api}}/v1/apps\n\n#  @name getApp\n"
		require.Equal(t, []string{"getApp"}, missingRequests(ops, requests),
			"запрос с неверной разметкой @name не считается, internal-операции не требуются")
	})

	t.Run("invalid spec fails validation", func(t *testing.T) {
		doc := loadSpecData(t, `
openapi: 3.0.3
info: { title: broken, version: 1.0.0 }
paths:
  /v1/apps:
    get:
      operationId: listApps
`)
		require.Error(t, doc.Validate(t.Context()), "операция без responses невалидна")
	})

	t.Run("unresolved ref fails loading", func(t *testing.T) {
		_, err := openapi3.NewLoader().LoadFromData([]byte(`
openapi: 3.0.3
info: { title: broken, version: 1.0.0 }
paths:
  /v1/apps:
    get:
      responses:
        '200': { $ref: '#/components/responses/Missing' }
`))
		require.Error(t, err)
	})
}
