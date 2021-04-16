package storage

import (
	"fmt"
)

type Context struct {
	Name      string
	Tenant    string
	TokenName string
}

type ContextConfig struct {
	CurrentContext    string
	AvailableContexts []Context
}

func (cfg *ContextConfig) AppendAvailableContext(c Context) {
	for i, context := range cfg.AvailableContexts {
		if c.Name == context.Name {
			cfg.AvailableContexts[i] = context
			return
		}
	}
	cfg.AvailableContexts = append(cfg.AvailableContexts, c)
}

type ContextConfigStore interface {
	Get() (*ContextConfig, error)
	Put(*ContextConfig) error
}

func CurrentContext(store ContextConfigStore) (*Context, error) {
	cfg, err := store.Get()
	if err != nil {
		return nil, err
	}

	if cfg.CurrentContext == "" {
		return nil, fmt.Errorf("current context has not been set")
	}

	for _, context := range cfg.AvailableContexts {
		if context.Name == cfg.CurrentContext {
			return &context, nil
		}
	}
	return nil, fmt.Errorf("current context does not exist")
}

func CurrentCredentials(ccStore ContextConfigStore, tokenStore TokenStore) (tenant, token, endpoint string, err error) {
	context, err := CurrentContext(ccStore)
	if err != nil {
		return "", "", "", err
	}

	t, err := tokenStore.Get(context.TokenName, false)
	if err != nil {
		return "", "", "", err
	}

	return context.Tenant, t.Token, t.Endpoint, nil
}
