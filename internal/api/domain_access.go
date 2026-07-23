package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

type domainAccessContextKey struct{}

// domainAccessScope is loaded once for each authenticated request. Admins are
// unrestricted; every other role is limited to its explicit domain_access rows.
// Both IDs and hostnames are retained because the API uses both forms.
type domainAccessScope struct {
	unrestricted bool
	domainIDs    map[string]struct{}
	domainNames  map[string]struct{}
}

func (h *Handlers) domainAccessScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetUserFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		scope, err := h.loadDomainAccessScope(r.Context(), claims)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load domain access")
			return
		}

		ctx := context.WithValue(r.Context(), domainAccessContextKey{}, scope)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handlers) loadDomainAccessScope(ctx context.Context, claims *auth.Claims) (domainAccessScope, error) {
	scope := domainAccessScope{unrestricted: claims.Role == "admin"}
	if scope.unrestricted {
		return scope, nil
	}

	scope.domainIDs = make(map[string]struct{})
	scope.domainNames = make(map[string]struct{})

	// Scope loading also protects the long-lived SSE route, which deliberately
	// cannot hold a read lock for the lifetime of the stream.
	h.dbMu.RLock()
	defer h.dbMu.RUnlock()

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT d.id, d.domain
		FROM domain_access da
		JOIN domains d ON d.id = da.domain_id
		WHERE da.user_id = ?
	`, claims.UserID)
	if err != nil {
		return domainAccessScope{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var domainID, domainName string
		if err := rows.Scan(&domainID, &domainName); err != nil {
			return domainAccessScope{}, err
		}
		scope.domainIDs[domainID] = struct{}{}
		scope.domainNames[domainName] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return domainAccessScope{}, err
	}
	return scope, nil
}

func domainScopeFromRequest(r *http.Request) (domainAccessScope, bool) {
	scope, ok := r.Context().Value(domainAccessContextKey{}).(domainAccessScope)
	return scope, ok
}

func (s domainAccessScope) allowsDomainID(domainID string) bool {
	if s.unrestricted {
		return true
	}
	_, ok := s.domainIDs[domainID]
	return ok
}

func (s domainAccessScope) allowsDomainName(domainName string) bool {
	if s.unrestricted {
		return true
	}
	_, ok := s.domainNames[domainName]
	return ok
}

type domainReferenceKind uint8

const (
	domainReferenceID domainReferenceKind = iota
	domainReferenceName
)

type domainReferenceExtractor func(*http.Request) (string, error)

func (h *Handlers) requireViewerDomainReference(kind domainReferenceKind, required bool, extract domainReferenceExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := domainScopeFromRequest(r)
			if !ok {
				writeError(w, http.StatusInternalServerError, "domain access scope unavailable")
				return
			}
			if scope.unrestricted {
				next.ServeHTTP(w, r)
				return
			}

			ref, err := extract(r)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusForbidden, "domain access denied")
				} else {
					writeError(w, http.StatusInternalServerError, "failed to verify domain access")
				}
				return
			}
			ref = strings.TrimSpace(ref)
			if ref == "" {
				if required {
					writeError(w, http.StatusForbidden, "a permitted domain is required")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			allowed := scope.allowsDomainID(ref)
			if kind == domainReferenceName {
				allowed = scope.allowsDomainName(ref)
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "domain access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handlers) requireViewerDomainQuery(param string, kind domainReferenceKind, required bool) func(http.Handler) http.Handler {
	return h.requireViewerDomainReference(kind, required, func(r *http.Request) (string, error) {
		return r.URL.Query().Get(param), nil
	})
}

func (h *Handlers) requireViewerDomainPath(param string, kind domainReferenceKind) func(http.Handler) http.Handler {
	return h.requireViewerDomainReference(kind, true, func(r *http.Request) (string, error) {
		return chi.URLParam(r, param), nil
	})
}

// requireViewerResourceDomain resolves an indirect route object (for example a
// replay session or tag-manager container) to its owning domain before the
// handler runs. resourceQuery is always a compile-time SQL string at call sites.
func (h *Handlers) requireViewerResourceDomain(pathParam string, kind domainReferenceKind, resourceQuery string) func(http.Handler) http.Handler {
	return h.requireViewerDomainReference(kind, true, func(r *http.Request) (string, error) {
		var domainRef string
		err := h.db.Conn().QueryRowContext(r.Context(), resourceQuery, chi.URLParam(r, pathParam)).Scan(&domainRef)
		return domainRef, err
	})
}

func restrictedDomainIDs(r *http.Request) ([]string, bool) {
	scope, ok := domainScopeFromRequest(r)
	if !ok || scope.unrestricted {
		return nil, false
	}
	values := make([]string, 0, len(scope.domainIDs))
	for value := range scope.domainIDs {
		values = append(values, value)
	}
	sort.Strings(values)
	return values, true
}

func restrictedDomainNames(r *http.Request) ([]string, bool) {
	scope, ok := domainScopeFromRequest(r)
	if !ok || scope.unrestricted {
		return nil, false
	}
	values := make([]string, 0, len(scope.domainNames))
	for value := range scope.domainNames {
		values = append(values, value)
	}
	sort.Strings(values)
	return values, true
}

// domainNameByID resolves a domains.id to its hostname, which is what the
// events.domain column stores. Queries filtering events by domain must bind
// the hostname, not the domain row id.
func (h *Handlers) domainNameByID(ctx context.Context, domainID string) (string, error) {
	var name string
	err := h.db.Conn().QueryRowContext(ctx,
		"SELECT domain FROM domains WHERE id = ?", domainID).Scan(&name)
	return name, err
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func stringsToAny(values []string) []any {
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = value
	}
	return args
}
