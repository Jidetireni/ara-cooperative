package email

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"sync"

	lru "github.com/hashicorp/golang-lru"
)

type EmailTemplateCache struct {
	dir string
	lru *lru.Cache // holds *template.Template
	mu  sync.Mutex
}

func NewEmailTemplateCache(dir string, capacity int) (*EmailTemplateCache, error) {
	lruCache, err := lru.New(capacity)
	if err != nil {
		return nil, err
	}
	return &EmailTemplateCache{
		dir: dir,
		lru: lruCache,
	}, nil
}

func (c *EmailTemplateCache) Get(name string) (*template.Template, error) {
	if v, ok := c.lru.Get(name); ok {
		return v.(*template.Template), nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// re-check under write lock
	if v, ok := c.lru.Get(name); ok {
		return v.(*template.Template), nil
	}
	path := filepath.Join(c.dir, name)
	tmpl, err := template.ParseFiles(path + ".html")
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.lru.Add(name, tmpl)
	return tmpl, nil
}

func (c *EmailTemplateCache) Render(name EmailTemplateType, data any) (string, error) {
	tmpl, err := c.Get(string(name))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("exec %s: %w", name, err)
	}
	return buf.String(), nil
}
