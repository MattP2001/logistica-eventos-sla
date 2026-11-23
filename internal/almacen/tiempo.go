package almacen

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/envios"
)

func fmtTiempo(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func fmtTiempoPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTiempo(*t)
}

func parseTiempo(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("fecha guardada invalida %q: %w", s, err)
	}
	return t, nil
}

func parseTiempoNull(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTiempo(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ordenarCronologico ordena por cuando ocurrio el evento; si dos ocurrieron al
// mismo tiempo, por cuando llegaron, y por ultimo por event_id para que el
// orden sea siempre el mismo.
func ordenarCronologico(evs []envios.Evento) {
	sort.SliceStable(evs, func(i, j int) bool {
		a, b := evs[i], evs[j]
		if !a.OcurridoEn.Equal(b.OcurridoEn) {
			return a.OcurridoEn.Before(b.OcurridoEn)
		}
		if !a.RecibidoEn.Equal(b.RecibidoEn) {
			return a.RecibidoEn.Before(b.RecibidoEn)
		}
		return a.EventID < b.EventID
	})
}
