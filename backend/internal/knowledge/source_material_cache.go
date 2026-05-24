package knowledge

import (
	"context"
	"errors"
	"time"
)

func (s *Service) readSourceMaterialStates(ctx context.Context, keys []SourceMaterialKey) (map[string]SourceMaterialState, []string) {
	states := map[string]SourceMaterialState{}
	if len(keys) == 0 {
		return states, nil
	}

	if s.cache != nil {
		cached, err := s.cache.ReadSourceMaterialStates(ctx, s.cfg.OwnerID, keys)
		if err == nil {
			for key, state := range cached {
				states[key] = state
			}
		} else if !errors.Is(err, ErrReadModelCacheMiss) {
			s.logger.Warn("source material cache fallback", "error", err)
		}
	}

	missing := make([]SourceMaterialKey, 0, len(keys))
	for _, key := range keys {
		if _, ok := states[key.String()]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return states, nil
	}

	canonical, err := s.store.ReadSourceMaterialStates(ctx, s.cfg.OwnerID, missing)
	if err != nil {
		return states, []string{"source material lookup failed: " + err.Error()}
	}
	for key, state := range canonical {
		states[key] = state
	}
	if len(canonical) > 0 {
		s.publishSourceMaterialStatesBestEffort(ctx, canonical)
	}
	return states, nil
}

func (s *Service) publishSourceMaterialStatesBestEffort(ctx context.Context, states map[string]SourceMaterialState) {
	if s.cache == nil || len(states) == 0 {
		return
	}
	values := make([]SourceMaterialState, 0, len(states))
	for _, state := range states {
		values = append(values, state)
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.cache.PublishSourceMaterialStates(cacheCtx, s.cfg.OwnerID, values); err != nil {
		s.logger.Warn("source material cache publish failed", "error", err)
	}
}
