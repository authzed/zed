package schemaexamples

import "embed"

//go:embed schemas/**/*.yaml
var exampleSchemas embed.FS

// ListExampleSchemas returns a list of all example schemas embedded in the binary.
func ListExampleSchemas() ([][]byte, error) {
	var schemas [][]byte
	found, err := exampleSchemas.ReadDir("schemas")
	if err != nil {
		return nil, err
	}

	for _, entry := range found {
		// Read the YAML found in each directory.
		if entry.IsDir() {
			subEntries, err := exampleSchemas.ReadDir("schemas/" + entry.Name())
			if err != nil {
				return nil, err
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					schema, err := exampleSchemas.ReadFile("schemas/" + entry.Name() + "/" + subEntry.Name())
					if err != nil {
						return nil, err
					}
					schemas = append(schemas, schema)
				}
			}
		}
	}
	return schemas, nil
}
