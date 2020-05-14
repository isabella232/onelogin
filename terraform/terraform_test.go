package terraform

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/onelogin/onelogin-go-sdk/pkg/models"
	"github.com/onelogin/onelogin-go-sdk/pkg/oltypes"
	"github.com/stretchr/testify/assert"
)

type MockFile struct {
	Content []byte
}

func (m *MockFile) Write(p []byte) (int, error) {
	m.Content = p
	return len(p), nil
}

func (m *MockFile) Read(p []byte) (int, error) {
	for i, b := range m.Content {
		p[i] = b
	}
	return len(p), nil
}

func TestFilterExistingDefinitions(t *testing.T) {
	tests := map[string]struct {
		InputFileCounts             map[string]map[string]int
		InputResourceDefinitions    []ResourceDefinition
		ExpectedResourceDefinitions []ResourceDefinition
		ExpectedProviders           []string
	}{
		"it yields lists of resource definitions and providers not already defined in main.tf": {
			InputFileCounts: map[string]map[string]int{
				"resource": map[string]int{
					"onelogin_apps.defined_in_main.tf_already": 1,
				},
				"provider": map[string]int{
					"onelogin": 1,
				},
			},
			InputResourceDefinitions: []ResourceDefinition{
				ResourceDefinition{
					Provider: "onelogin",
					Name:     "defined_in_main.tf_already",
					Type:     "onelogin_apps",
				},
				ResourceDefinition{
					Provider: "onelogin",
					Name:     "new_resource",
					Type:     "onelogin_apps",
				},
				ResourceDefinition{
					Provider: "onelogin",
					Name:     "test",
					Type:     "onelogin_saml_apps",
				},
				ResourceDefinition{
					Provider: "okra",
					Name:     "test",
					Type:     "okra_saml_apps",
				},
			},
			ExpectedResourceDefinitions: []ResourceDefinition{
				ResourceDefinition{
					Provider: "onelogin",
					Name:     "new_resource",
					Type:     "onelogin_apps",
				},
				ResourceDefinition{
					Provider: "onelogin",
					Name:     "test",
					Type:     "onelogin_saml_apps",
				},
				ResourceDefinition{
					Provider: "okra",
					Name:     "test",
					Type:     "okra_saml_apps",
				},
			},
			ExpectedProviders: []string{"okra"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actualResourceDefinitions, actualProviderDefinitions := filterExistingDefinitions(test.InputFileCounts, test.InputResourceDefinitions)
			assert.Equal(t, test.ExpectedResourceDefinitions, actualResourceDefinitions)
			assert.Equal(t, test.ExpectedProviders, actualProviderDefinitions)
		})
	}
}

func TestCollectTerraformDefinitionsFromFile(t *testing.T) {
	tests := map[string]struct {
		InputReadWriter     io.Reader
		ExpectedDefinitions map[string]map[string]int
	}{
		"it finds resource and provider definitions in main.tf": {
			InputReadWriter: strings.NewReader(`
				provider onelogin {
					alias = "onelogin"
				}
				resource onelogin_apps test {
					name = "should not be here"
				}
				resource onelogin_apps test {
					name = "this is not proper HCL and will get counted again"
				}
			`),
			ExpectedDefinitions: map[string]map[string]int{
				"resource": map[string]int{
					"onelogin_apps.test": 2,
				},
				"provider": map[string]int{
					"onelogin": 1,
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := collectTerraformDefinitionsFromFile(test.InputReadWriter)
			assert.Equal(t, test.ExpectedDefinitions, actual)
		})
	}
}

func TestConvertTFStateToHCL(t *testing.T) {
	tests := map[string]struct {
		InputState     State
		ExpectedOutput []byte
	}{
		"it creates a bytes buffer representing the state in HCL": {
			InputState: State{
				Resources: []StateResource{
					StateResource{
						Name:     "test_resource",
						Type:     "onelogin_apps",
						Provider: "provider.onelogin",
						Instances: []ResourceInstance{
							ResourceInstance{
								Data: ResourceData{
									Name: oltypes.String("test"),
									Provisioning: []models.AppProvisioning{
										models.AppProvisioning{
											Enabled: oltypes.Bool(true),
										},
									},
								},
							},
						},
					},
				},
			},
			ExpectedOutput: []byte("provider onelogin {\n\talias = \"onelogin\"\n}\n\nresource onelogin_apps test_resource {\n\tprovider = onelogin\n\tname = \"test\"\n\n\tprovisioning {\n\t\tenabled = true\n\t}\n}\n\n"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := convertTFStateToHCL(test.InputState)
			assert.Equal(t, test.ExpectedOutput, actual)
		})
	}
}

func TestResourceBaseToHCL(t *testing.T) {
	tests := map[string]struct {
		InputInstance ResourceData
		ExpectedOut   []byte
	}{
		"it creates a bytes buffer representing formatted HCL": {
			InputInstance: ResourceData{
				Name: oltypes.String("test"),
				Provisioning: []models.AppProvisioning{
					models.AppProvisioning{
						Enabled: oltypes.Bool(true),
					},
				},
			},
			ExpectedOut: []byte("\tname = \"test\"\n\n\tprovisioning {\n\t\tenabled = true\n\t}\n"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := convertToHCLByteSlice(test.InputInstance, 1)
			assert.Equal(t, test.ExpectedOut, actual)
		})
	}
}

func TestAppendDefinitionsToMainTF(t *testing.T) {
	tests := map[string]struct {
		InputWriter              io.ReadWriter
		InputResourceDefinitions []ResourceDefinition
		InputProviderDefinitions []string
		ExpectedOut              []byte
	}{
		"it adds provider and resource to the writer": {
			InputWriter: &MockFile{},
			InputResourceDefinitions: []ResourceDefinition{
				ResourceDefinition{
					Content:  []byte{},
					Name:     "test",
					Type:     "test",
					Provider: "test",
				},
				ResourceDefinition{
					Content:  []byte{},
					Name:     "test",
					Type:     "test",
					Provider: "test2",
				},
			},
			InputProviderDefinitions: []string{"test", "test2"},
			ExpectedOut:              []byte(fmt.Sprintf("provider test {\n\talias = \"test\"\n}\n\nprovider test2 {\n\talias = \"test2\"\n}\n\nresource test test {}\nresource test test {}\n")),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := make([]byte, len(test.ExpectedOut))
			appendDefinitionsToMainTF(test.InputWriter, test.InputResourceDefinitions, test.InputProviderDefinitions)
			test.InputWriter.Read(actual)
			assert.Equal(t, test.ExpectedOut, actual)
		})
	}
}