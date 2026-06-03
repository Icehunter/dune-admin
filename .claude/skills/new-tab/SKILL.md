---
name: new-tab
description: Scaffold a new full-stack tab for dune-admin — Go route+handler+db func, api/client.ts namespace, and React tab component following the BasesTab pattern.
---

# New Tab Scaffold — dune-admin

Read `web/src/tabs/BasesTab.tsx` before starting. It is the canonical reference.

## Step-by-step

### 1. Backend: route in `server.go`

```go
mux.HandleFunc("GET /api/v1/foos",     handleGetFoos)
mux.HandleFunc("GET /api/v1/foos/{id}", handleGetFoo)
```

### 2. Backend: handler in `handlers_foos.go`

Follow the standard pattern — guard globalDB, call cmd* from db.go, use jsonOK/jsonErr.

Write the `*_test.go` file **first** (see `tdd-go` skill).

### 3. Backend: query in `db.go`

```go
func cmdFetchFoos(ctx context.Context, db *pgxpool.Pool) ([]fooRow, error) {
    rows, err := db.Query(ctx, `SELECT id, name FROM dune.foos ORDER BY name`)
    // ...scan rows...
}
```

Always use `dune.` schema prefix.

### 4. Frontend: API namespace in `web/src/api/client.ts`

Add to the `api` export object:

```ts
foos: {
    list: () => req<FooRow[]>('GET', '/api/v1/foos'),
    get:  (id: number) => req<FooRow>('GET', `/api/v1/foos/${id}`),
},
```

Add the `FooRow` type nearby or in a `types.ts` if complex.

### 5. Frontend: tab component

**Simple tab** → single file `web/src/tabs/FoosTab.tsx`:

```tsx
export default function FoosTab() {
  const [data, setData] = useState<FooRow[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      setData(await api.foos.list())
    } catch (e) {
      toast.danger(`Failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const columns: Column<FooRow>[] = [
    { key: 'id',   label: 'ID',   sortable: true },
    { key: 'name', label: 'Name', sortable: true },
  ]

  return (
    <Panel>
      <PageHeader title="Foos" onRefresh={load} loading={loading} />
      <DataTable columns={columns} rows={data} loading={loading} rowKey="id" />
    </Panel>
  )
}
```

**Complex tab** → directory `web/src/tabs/FoosTab/index.tsx` + `types.ts` + `components/` + `modals/`.

### 6. Register the tab in `web/src/App.tsx`

Add to the tab list and import the component.

### 7. Verify

```bash
make test-race          # backend tests pass
cd web && pnpm lint     # no new ESLint errors
cd web && pnpm build    # tsc + vite clean
make verify             # full suite
```

## Checklist

- [ ] Handler test written first (red → green)
- [ ] Route registered in server.go
- [ ] SQL in db.go with dune. prefix
- [ ] globalDB guard in handler
- [ ] api/client.ts namespace added
- [ ] Tab follows BasesTab pattern
- [ ] Semantic colour tokens only (no inline style, no raw Tailwind colours)
- [ ] pnpm lint + build clean
- [ ] make verify passes
