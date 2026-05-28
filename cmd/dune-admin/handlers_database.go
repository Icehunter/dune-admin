package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
	sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlReadOnlyRe   = regexp.MustCompile(`^(select|explain|show|with)[\s(]`)
)

func isReadOnlySQL(sql string) bool {
	s := sqlBlockComment.ReplaceAllString(sql, " ")
	s = sqlLineComment.ReplaceAllString(s, " ")
	s = strings.ToLower(strings.TrimSpace(s))
	return sqlReadOnlyRe.MatchString(s)
}

func handleDBTables(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchTables().(msgTables)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	type tableOut struct {
		Name     string `json:"name"`
		RowCount int64  `json:"row_count"`
	}
	rows := make([]tableOut, 0, len(msg.rows))
	for _, r := range msg.rows {
		rows = append(rows, tableOut(r))
	}
	jsonOK(w, rows)
}

func handleDBDescribe(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		jsonErr(w, fmt.Errorf("table required"), 400)
		return
	}
	msg, ok := cmdDescribeTable(table)().(msgDescribe)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	type colOut struct {
		Name     string `json:"name"`
		DataType string `json:"data_type"`
		Nullable string `json:"nullable"`
	}
	cols := make([]colOut, 0, len(msg.cols))
	for _, c := range msg.cols {
		cols = append(cols, colOut(c))
	}
	jsonOK(w, map[string]any{"table": msg.table, "columns": cols})
}

func handleDBSample(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		jsonErr(w, fmt.Errorf("table required"), 400)
		return
	}
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 20
	}
	msg, ok := cmdSampleTable(table, limit)().(msgSample)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{
		"table":   msg.table,
		"headers": msg.headers,
		"rows":    msg.rows,
	})
}

func handleDBSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		jsonErr(w, fmt.Errorf("term required"), 400)
		return
	}
	msg, ok := cmdSearchColumns(term)().(msgSearchCols)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{
		"headers": msg.headers,
		"rows":    msg.rows,
	})
}

func handleDBSQL(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		SQL string `json:"sql"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.SQL == "" {
		jsonErr(w, fmt.Errorf("sql required"), 400)
		return
	}
	if !isReadOnlySQL(req.SQL) {
		jsonErr(w, fmt.Errorf("only SELECT, EXPLAIN, and SHOW statements are allowed"), 400)
		return
	}
	msg, ok := cmdRunSQL(req.SQL)().(msgSQL)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"result": msg.result})
}
