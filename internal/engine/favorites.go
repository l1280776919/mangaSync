package engine

import (
	"context"
	"errors"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
	"time"
)

type favoriteLoad struct {
	done  chan struct{}
	items []*source.Comic
	err   error
}

func (e *Engine) favoriteLoad(id int64) *favoriteLoad {
	e.favMu.Lock()
	defer e.favMu.Unlock()
	if f := e.favoriteLoads[id]; f != nil {
		return f
	}
	f := &favoriteLoad{done: make(chan struct{})}
	e.favoriteLoads[id] = f
	e.syncMu.Lock()
	root := e.lifeCtx
	e.syncMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(root, 10*time.Minute)
		defer cancel()
		defer func() { e.favMu.Lock(); delete(e.favoriteLoads, id); close(f.done); e.favMu.Unlock() }()
		if f.err = e.st.BeginFavorites(id); f.err != nil {
			return
		}
		acc, err := e.st.GetAccount(id)
		if err == nil {
			var src source.Source
			src, err = e.SourceFor(acc.Kind)
			if err == nil {
				var cred *source.Cred
				cred, err = e.EnsureCred(ctx, acc)
				if err == nil {
					f.items, err = src.Favorites(ctx, cred)
					if errors.Is(err, source.ErrAuth) {
						cred, err = e.Relogin(ctx, acc)
						if err == nil {
							f.items, err = src.Favorites(ctx, cred)
						}
					}
				}
			}
		}
		f.err = err
		if saveErr := e.st.SaveFavorites(id, f.items, err); saveErr != nil {
			f.err = errors.Join(err, saveErr)
		}
	}()
	return f
}
func (e *Engine) RefreshFavorites(ctx context.Context, id int64) ([]*source.Comic, error) {
	f := e.favoriteLoad(id)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return f.items, f.err
	}
}
func (e *Engine) CachedFavorites(id int64, keyword string, page, size int, force bool) ([]*source.Comic, int, store.FavoriteState, bool, error) {
	state, err := e.st.FavoriteState(id)
	if err != nil {
		return nil, 0, state, false, err
	}
	updated, _ := time.Parse(time.RFC3339, state.UpdatedAt)
	attempt, _ := time.Parse(time.RFC3339, state.AttemptAt)
	if force || (time.Since(updated) > 5*time.Minute && time.Since(attempt) > time.Minute) {
		e.favoriteLoad(id)
	}
	e.favMu.Lock()
	refreshing := e.favoriteLoads[id] != nil
	e.favMu.Unlock()
	items, total, err := e.st.FavoritePage(id, keyword, page, size)
	return items, total, state, refreshing, err
}
