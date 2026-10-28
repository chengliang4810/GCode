package contracts

type ModelCatalogStatus string

const (
	CatalogReady    ModelCatalogStatus = "ready"
	CatalogLoading  ModelCatalogStatus = "loading"
	CatalogError    ModelCatalogStatus = "error"
	CatalogEmpty    ModelCatalogStatus = "empty"
	CatalogNoAgents ModelCatalogStatus = "noAgents"
)

type ModelCatalogState struct {
	State ModelCatalogStatus `json:"state"`
	Error string             `json:"error,omitempty"`
}
