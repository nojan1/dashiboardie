package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// listTimeout bounds each provider so one hung CLI cannot stall the dashboard.
const listTimeout = 10 * time.Second

// Registry aggregates all providers into a single list.
type Registry struct {
	all       []Provider
	providers map[Source]Provider // routing for actions and logs
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{all: providers, providers: make(map[Source]Provider)}
	for _, p := range providers {
		for _, src := range p.Sources() {
			r.providers[src] = p
		}
	}
	return r
}

// ProviderError reports that one source failed to list; the others still show.
type ProviderError struct {
	Source Source `json:"source"`
	Error  string `json:"error"`
}

// List queries every provider concurrently, merges the results, and removes
// systemd entries whose unit is already represented by a Quadlet service.
func (r *Registry) List(ctx context.Context) ([]Service, []ProviderError) {
	type result struct {
		src  Source
		svcs []Service
		err  error
	}
	results := make(chan result, len(r.all))
	var wg sync.WaitGroup
	for _, p := range r.all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, listTimeout)
			defer cancel()
			svcs, err := p.List(ctx)
			results <- result{p.Sources()[0], svcs, err}
		}()
	}
	wg.Wait()
	close(results)

	var all []Service
	var errs []ProviderError
	for res := range results {
		if res.err != nil {
			errs = append(errs, ProviderError{Source: res.src, Error: res.err.Error()})
			continue
		}
		all = append(all, res.svcs...)
	}

	all = dedupeQuadlets(all)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		return all[i].Name < all[j].Name
	})
	sort.Slice(errs, func(i, j int) bool { return errs[i].Source < errs[j].Source })
	return all, errs
}

func dedupeQuadlets(svcs []Service) []Service {
	quadletUnits := make(map[string]bool)
	for _, s := range svcs {
		if s.Source == SourceQuadlet && s.Unit != "" {
			quadletUnits[s.Unit] = true
		}
	}
	out := svcs[:0]
	for _, s := range svcs {
		if s.Source == SourceSystemd && quadletUnits[s.Unit] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (r *Registry) provider(src Source) (Provider, error) {
	p, ok := r.providers[src]
	if !ok {
		return nil, fmt.Errorf("unknown source %q: %w", src, ErrNotFound)
	}
	return p, nil
}

func (r *Registry) Do(ctx context.Context, src Source, id string, action Action) error {
	p, err := r.provider(src)
	if err != nil {
		return err
	}
	return p.Do(ctx, src, id, action)
}

func (r *Registry) Logs(ctx context.Context, src Source, id string, lines int) ([]string, error) {
	p, err := r.provider(src)
	if err != nil {
		return nil, err
	}
	return p.Logs(ctx, src, id, lines)
}
