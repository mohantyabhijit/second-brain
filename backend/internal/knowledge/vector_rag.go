package knowledge

import (
	"context"
	"fmt"
	"time"
)

type askVectorStore interface {
	SearchAskSources(ctx context.Context, ownerID string, model string, queryVector string, queryText string, limit int) ([]AskSecondBrainSource, error)
}

func (s *Service) retrieveVectorSources(ctx context.Context, question string, limit int) []AskSecondBrainSource {
	store, ok := s.store.(askVectorStore)
	if !ok {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	vectors := s.embeddingLiterals(queryCtx, []string{question})
	if len(vectors) == 0 || vectors[0] == "" {
		return nil
	}
	sources, err := store.SearchAskSources(queryCtx, s.cfg.OwnerID, s.embeddingModel(), vectors[0], question, limit)
	if err != nil {
		s.log(ctx).Warn("pgvector rag query failed", "error", err)
		return nil
	}
	for index := range sources {
		if sources[index].ID == "" {
			sources[index].ID = fmt.Sprintf("V%d", index+1)
		}
	}
	return sources
}
