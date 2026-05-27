package marketbot

import (
	"context"
	"sort"
)

// reportData returns per-item sales rows for the API /report endpoint.
func (e *Exchange) reportData(ctx context.Context) []reportRow {
	if e.db == nil {
		return nil
	}
	rows, err := e.db.Query(ctx, `
		SELECT o.template_id,
		       COALESCE(SUM(f.stack_size), 0)          AS sold,
		       COALESCE(MAX(s.initial_stack_size), 0)  AS listed
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.dune_exchange_fulfilled_orders f ON f.order_id = o.id
		WHERE o.owner_id = $1 AND o.is_npc_order = TRUE
		GROUP BY o.template_id
		ORDER BY o.template_id`, e.ownerID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var report []reportRow
	for rows.Next() {
		var tmpl string
		var sold, listed int64
		if err := rows.Scan(&tmpl, &sold, &listed); err != nil {
			continue
		}
		var sellPct float64
		if listed > 0 {
			sellPct = float64(sold) / float64(listed) * 100
		}
		item := e.catalogMap[tmpl]
		report = append(report, reportRow{
			Template: tmpl,
			Sold:     sold,
			Listed:   listed,
			SellPct:  sellPct,
			Price:    e.prices[tmpl],
			MinPrice: item.MinPrice,
			MaxPrice: item.MaxPrice,
			Buyable:  item.Buyable,
		})
	}

	sort.Slice(report, func(i, j int) bool {
		return report[i].Template < report[j].Template
	})

	return report
}

type reportRow struct {
	Template string  `json:"template"`
	Sold     int64   `json:"sold"`
	Listed   int64   `json:"listed"`
	SellPct  float64 `json:"sell_pct"`
	Price    int64   `json:"price"`
	MinPrice int64   `json:"min_price"`
	MaxPrice int64   `json:"max_price"`
	Buyable  bool    `json:"buyable"`
}
