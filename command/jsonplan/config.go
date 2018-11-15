package jsonplan

import (
	"github.com/hashicorp/terraform/configs"
	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/terraform"
)

// Config represents the complete configuration source
type config struct {
	ProviderConfigs []providerConfig `json:"provider_config,omitempty"`
	RootModule      configModule     `json:"root_module,omitempty"`
}

// ProviderConfig describes all of the provider configurations throughout the
// configuration tree, flattened into a single map for convenience since
// provider configurations are the one concept in Terraform that can span across
// module boundaries.
type providerConfig struct {
	Name          string                 `json:"name,omitempty"`
	Alias         string                 `json:"alias,omitempty"`
	ModuleAddress string                 `json:"module_address,omitempty"`
	Expressions   map[string]interface{} `json:"expressions,omitempty"`
}

type configModule struct {
	Outputs     map[string]configOutput `json:"outputs,omitempty"`
	Resources   []configResource        `json:"resources,omitempty"`
	ModuleCalls []moduleCall            `json:"module_calls,omitempty"`
}

// Resource is the representation of a resource in the config
type configResource struct {
	// Address is the absolute resource address
	Address string `json:"address,omitempty"`

	// Mode can be "managed" or "data"
	Mode string `json:"mode,omitempty"`

	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`

	// ProviderConfigKey is the key into "provider_configs" (shown above) for
	// the provider configuration that this resource is associated with.
	ProviderConfigKey string `json:"provider_config_key,omitempty"`

	// Provisioners is an optional field which describes any provisioners.
	// Connection info will not be included here.
	Provisioners []provisioner `json:"provisioners,omitempty"`

	// Expressions" describes the resource-type-specific  content of the
	// configuration block.
	Expressions map[string]interface{} `json:"expressions,omitempty"`
	// SchemaVersion indicates which version of the resource type schema the
	// "values" property conforms to.
	SchemaVersion uint64 `json:"schema_version,omitempty"`

	// CountExpression and ForEachExpression describe the expressions given for
	// the corresponding meta-arguments in the resource configuration block.
	// These are omitted if the corresponding argument isn't set.
	CountExpression   expression `json:"count_expression,omitempty"`
	ForEachExpression expression `json:"for_each_expression,omitempty"`
}

type configOutput struct {
	Sensitive  bool       `json:"sensitive,omitempty"`
	Expression expression `json:"expression,omitempty"`
}

type provisioner struct {
	Name        string                 `json:"name,omitempty"`
	Expressions map[string]interface{} `json:"expressions,omitempty"`
}

func (p *plan) marshalConfig(snap *configload.Snapshot, schemas *terraform.Schemas) error {
	configLoader := configload.NewLoaderFromSnapshot(snap)
	c, diags := configLoader.LoadConfig(snap.Modules[""].Dir)
	if diags.HasErrors() {
		return diags
	}

	// FIXME: this is not accurate provider marshaling, just a placeholder
	var pcs []providerConfig
	providers := c.ProviderTypes()
	for p := range providers {
		pc := providerConfig{
			Name: providers[p],
		}
		pcs = append(pcs, pc)
	}
	p.Config.ProviderConfigs = pcs
	p.Config.RootModule = marshalRootModule(c.Module, schemas)

	return nil
}

func marshalRootModule(m *configs.Module, schemas *terraform.Schemas) configModule {
	var module configModule

	var rs []configResource

	// NOTE TO REVIEWERS:
	// This is precisely where I started to worry.
	rs = marshalConfigResources(m.ManagedResources, schemas)
	rs = append(rs, marshalConfigResources(m.DataResources, schemas)...)
	module.Resources = rs

	outputs := make(map[string]configOutput)
	for _, v := range m.Outputs {
		outputs[v.Name] = configOutput{
			Sensitive:  v.Sensitive,
			Expression: marshalExpression(v.Expr),
		}
	}
	module.Outputs = outputs

	var mcs []moduleCall
	for _, v := range m.ModuleCalls {
		mc := moduleCall{
			ResolvedSource: v.SourceAddr,
		}
		cExp := marshalExpression(v.Count)
		if !cExp.Empty() {
			mc.CountExpression = cExp
		} else {
			fExp := marshalExpression(v.ForEach)
			if !fExp.Empty() {
				mc.ForEachExpression = fExp
			}
		}

		// schema = schemas.
		// mc.Expressions = marshalExpressions(v.Config, schema)
		mcs = append(mcs, mc)
	}

	module.ModuleCalls = mcs
	return module
}

func marshalConfigResources(resources map[string]*configs.Resource, schemas *terraform.Schemas) []configResource {
	var rs []configResource
	for _, v := range resources {
		r := configResource{
			Address:           v.Addr().String(),
			Mode:              v.Mode.String(),
			Type:              v.Type,
			Name:              v.Name,
			ProviderConfigKey: v.ProviderConfigAddr().String(),
		}

		cExp := marshalExpression(v.Count)
		if !cExp.Empty() {
			r.CountExpression = cExp
		} else {
			fExp := marshalExpression(v.ForEach)
			if !fExp.Empty() {
				r.ForEachExpression = fExp
			}
		}

		schema, schemaVersion := schemas.ResourceTypeConfig(v.ProviderConfigAddr().String(), v.Mode, v.Type)
		r.SchemaVersion = schemaVersion

		r.Expressions = marshalExpressions(v.Config, schema)

		var provisioners []provisioner
		for _, p := range v.Managed.Provisioners {
			schema := schemas.ProvisionerConfig(p.Type)
			prov := provisioner{
				Name:        p.Type,
				Expressions: marshalExpressions(p.Config, schema),
			}
			provisioners = append(provisioners, prov)
		}
		r.Provisioners = provisioners

		rs = append(rs, r)
	}
	return rs
}
