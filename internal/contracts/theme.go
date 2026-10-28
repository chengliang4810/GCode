package contracts

type ThemeLibraryEntry struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	SourceKind string   `json:"sourceKind"`
	Path       string   `json:"path"`
	VariantIDs []string `json:"variantIds"`
	Warning    string   `json:"warning"`
}
type ThemeImportPreview struct {
	Path     string               `json:"path"`
	Mode     string               `json:"mode"`
	Variants []ThemeImportVariant `json:"variants"`
	Warnings []string             `json:"warnings"`
	Error    string               `json:"error"`
	Ready    bool                 `json:"ready"`
}
type ThemeImportVariant struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Appearance string `json:"appearance"`
	Selected   bool   `json:"selected"`
}
