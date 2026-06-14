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

// @Summary List all tables in the dune schema
// @Tags database
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/database/tables [get]
func handleDBTables(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchTables(dbFromCtx(r)).(msgTables)
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

// @Summary Describe columns of a table
// @Tags database
// @Produce json
// @Param table query string true "Table name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/database/describe [get]
func handleDBDescribe(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		jsonErr(w, fmt.Errorf("table required"), 400)
		return
	}
	msg, ok := cmdDescribeTable(dbFromCtx(r), table)().(msgDescribe)
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

// @Summary Return sample rows from a table
// @Tags database
// @Produce json
// @Param table query string true "Table name"
// @Param limit query int false "Number of rows to return (default 20, max 500)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/database/sample [get]
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
	msg, ok := cmdSampleTable(dbFromCtx(r), table, limit)().(msgSample)
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

// @Summary Search for a term across all table columns
// @Tags database
// @Produce json
// @Param term query string true "Search term"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/database/search [get]
func handleDBSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		jsonErr(w, fmt.Errorf("term required"), 400)
		return
	}
	msg, ok := cmdSearchColumns(dbFromCtx(r), term)().(msgSearchCols)
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

// @Summary Execute a read-only SQL query (SELECT/EXPLAIN/SHOW only)
// @Tags database
// @Accept json
// @Produce json
// @Param body body object true "SQL query" SchemaExample({"sql": "SELECT 1"})
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/database/sql [post]
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
	msg, ok := cmdRunSQL(dbFromCtx(r), req.SQL)().(msgSQL)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{
		"headers":   msg.headers,
		"rows":      msg.rows,
		"truncated": msg.truncated,
	})
}
