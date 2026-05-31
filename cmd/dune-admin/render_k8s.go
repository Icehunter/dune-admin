package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func yamlScalar(s string) string {
	b, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSpace(string(b))
}

func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}

func addIfNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func addAmpManifestFields(m map[string]any, cfg *appConfig, control string) {
	if cfg.Control != "amp" && control != "amp" {
		return
	}
	addIfNonEmpty(m, "amp_instance", cfg.AmpInstance)
	addIfNonEmpty(m, "amp_container", cfg.AmpContainer)
	addIfNonEmpty(m, "amp_container_runtime", cfg.AmpContainerRuntime)
	addIfNonEmpty(m, "amp_user", cfg.AmpUser)
	addIfNonEmpty(m, "amp_log_path", cfg.AmpLogPath)
	if cfg.AmpUseContainer != nil {
		m["amp_use_container"] = *cfg.AmpUseContainer
	}
	addIfNonEmpty(m, "amp_data_root", cfg.AmpDataRoot)
	addIfNonEmpty(m, "director_url", cfg.DirectorURL)
}

func renderK8SManifest(outPath string) error {
	control := firstNonEmpty(controlPlane, loadedConfig.Control)
	if control == "" {
		control = resolveControl()
	}
	listen := firstNonEmpty(listenAddr, loadedConfig.ListenAddr)
	if listen == "" {
		listen = ":8080"
	}

	dbHostVal := firstNonEmpty(dbHost, loadedConfig.DBHost, "postgres.default.svc.cluster.local")
	dbPortVal := dbPort
	if dbPortVal == 0 {
		dbPortVal = loadedConfig.DBPort
	}
	if dbPortVal == 0 {
		dbPortVal = 15432
	}
	dbUserVal := firstNonEmpty(dbUser, loadedConfig.DBUser, "dune")
	dbNameVal := firstNonEmpty(dbName, loadedConfig.DBName, "dune")
	dbSchemaVal := firstNonEmpty(dbSchema, loadedConfig.DBSchema, "dune")
	dbPassVal := firstNonEmpty(dbPass, loadedConfig.DBPass, "replace-me")

	buyInt := parseDurString(loadedConfig.MarketBotBuyInt, 5*time.Minute)
	listInt := parseDurString(loadedConfig.MarketBotListInt, 30*time.Minute)
	cacheDB := firstNonEmpty(loadedConfig.MarketBotCacheDB, "/data/market-bot-cache.db")
	itemData := firstNonEmpty(loadedConfig.MarketBotItemData, "/app/item-data.json")
	buyThreshold := loadedConfig.MarketBotThresh
	if buyThreshold == 0 {
		buyThreshold = 1.05
	}
	maxBuys := loadedConfig.MarketBotMaxBuys
	if maxBuys == 0 {
		maxBuys = 50
	}

	manifestCfg := map[string]any{
		"control":                  control,
		"listen_addr":              listen,
		"db_host":                  dbHostVal,
		"db_port":                  dbPortVal,
		"db_user":                  dbUserVal,
		"db_name":                  dbNameVal,
		"db_schema":                dbSchemaVal,
		"market_bot_enabled":       marketBotEnabled(loadedConfig),
		"market_bot_cache_db":      cacheDB,
		"market_bot_item_data":     itemData,
		"market_bot_buy_interval":  buyInt.String(),
		"market_bot_list_interval": listInt.String(),
		"market_bot_buy_threshold": buyThreshold,
		"market_bot_max_buys":      maxBuys,
	}
	addIfNonEmpty(manifestCfg, "market_bot_remote_url", loadedConfig.MarketBotRemoteURL)
	addIfNonEmpty(manifestCfg, "ssh_host", loadedConfig.SSHHost)
	addIfNonEmpty(manifestCfg, "ssh_user", loadedConfig.SSHUser)
	addIfNonEmpty(manifestCfg, "ssh_key", loadedConfig.SSHKey)
	addIfNonEmpty(manifestCfg, "control_namespace", loadedConfig.ControlNamespace)
	addIfNonEmpty(manifestCfg, "broker_game_addr", loadedConfig.BrokerGameAddr)
	addIfNonEmpty(manifestCfg, "broker_admin_addr", loadedConfig.BrokerAdminAddr)
	if loadedConfig.BrokerTLS {
		manifestCfg["broker_tls"] = true
	}
	addIfNonEmpty(manifestCfg, "server_ini_dir", loadedConfig.ServerIniDir)
	addIfNonEmpty(manifestCfg, "default_ini_dir", loadedConfig.DefaultIniDir)
	addIfNonEmpty(manifestCfg, "backup_dir", loadedConfig.BackupDir)
	addAmpManifestFields(manifestCfg, &loadedConfig, control)

	cfgYAMLBytes, err := yaml.Marshal(manifestCfg)
	if err != nil {
		return fmt.Errorf("marshal embedded config: %w", err)
	}

	brokerUserVal := firstNonEmpty(brokerUser, loadedConfig.BrokerUser)
	brokerPassVal := firstNonEmpty(brokerPass, loadedConfig.BrokerPass)
	brokerJWTVal := firstNonEmpty(os.Getenv("BROKER_JWT_SECRET"), loadedConfig.BrokerJWTSecret)

	var out strings.Builder
	out.WriteString("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: dune-admin\n---\n")
	out.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: dune-admin-config\n  namespace: dune-admin\n")
	out.WriteString("data:\n")
	out.WriteString("  DB_HOST: " + yamlScalar(dbHostVal) + "\n")
	out.WriteString("  DB_PORT: " + yamlScalar(strconv.Itoa(dbPortVal)) + "\n")
	out.WriteString("  DB_USER: " + yamlScalar(dbUserVal) + "\n")
	out.WriteString("  DB_NAME: " + yamlScalar(dbNameVal) + "\n")
	out.WriteString("  DB_SCHEMA: " + yamlScalar(dbSchemaVal) + "\n")
	out.WriteString("  LISTEN_ADDR: " + yamlScalar(listen) + "\n")
	out.WriteString("  CONTROL: " + yamlScalar(control) + "\n")
	out.WriteString("  MARKET_BOT_ENABLED: " + yamlScalar(strconv.FormatBool(marketBotEnabled(loadedConfig))) + "\n")
	out.WriteString("  MARKET_BOT_BUY_INTERVAL: " + yamlScalar(buyInt.String()) + "\n")
	out.WriteString("  MARKET_BOT_LIST_INTERVAL: " + yamlScalar(listInt.String()) + "\n")
	out.WriteString("  config.yaml: |\n")
	out.WriteString(indentLines(string(cfgYAMLBytes), "    "))

	out.WriteString("---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: dune-admin-secrets\n  namespace: dune-admin\n")
	out.WriteString("type: Opaque\nstringData:\n")
	out.WriteString("  DB_PASS: " + yamlScalar(dbPassVal) + "\n")
	out.WriteString("  BROKER_USER: " + yamlScalar(brokerUserVal) + "\n")
	out.WriteString("  BROKER_PASS: " + yamlScalar(brokerPassVal) + "\n")
	out.WriteString("  BROKER_JWT_SECRET: " + yamlScalar(brokerJWTVal) + "\n")
	if loadedConfig.MarketBotRemoteToken != "" {
		out.WriteString("  MARKET_BOT_REMOTE_TOKEN: " + yamlScalar(loadedConfig.MarketBotRemoteToken) + "\n")
	}

	out.WriteString(`---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: market-bot-cache
  namespace: dune-admin
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
# Persistent volume for the dune-admin binary and frontend assets.
# seed-binary init container populates this on first deploy; subsequent restarts
# use whatever binary is in the PVC, including binaries placed by self-update.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: dune-admin-app
  namespace: dune-admin
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 500Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dune-admin
  namespace: dune-admin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dune-admin
  template:
    metadata:
      labels:
        app: dune-admin
    spec:
      initContainers:
        - name: seed-binary
          image: ghcr.io/icehunter/dune-admin:latest
          imagePullPolicy: IfNotPresent
          command:
            - sh
            - -c
            - |
              set -e
              STORED=$(cat /app/.image-build-time 2>/dev/null || echo "")
              if [ ! -f /app/dune-admin ] || [ "$IMAGE_BUILD_TIME" != "$STORED" ]; then
                echo "Seeding dune-admin binary and assets (built: $IMAGE_BUILD_TIME)..."
                cp /usr/local/share/dune-admin-seed/tags-data.json /usr/local/share/dune-admin-seed/item-data.json /app/
                rm -rf /app/dist && cp -r /usr/local/share/dune-admin-seed/dist /app/dist
                cp /usr/local/share/dune-admin-seed/dune-admin /app/.dune-admin.new
                chmod 0755 /app/.dune-admin.new
                mv /app/.dune-admin.new /app/dune-admin
                echo "$IMAGE_BUILD_TIME" > /app/.image-build-time
                echo "Seed complete."
              else
                echo "PVC already has this build ($IMAGE_BUILD_TIME) — skipping seed."
              fi
          volumeMounts:
            - name: app-rw
              mountPath: /app
        - name: seed-config
          image: busybox:latest
          command: ['sh', '-c', 'cp /configmap/config.yaml /app-config/config.yaml']
          volumeMounts:
            - name: config-source
              mountPath: /configmap
              readOnly: true
            - name: config-rw
              mountPath: /app-config
      containers:
        - name: dune-admin
          image: ghcr.io/icehunter/dune-admin:latest
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
              name: http
          env:
            - name: DB_HOST
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: DB_HOST
            - name: DB_PORT
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: DB_PORT
            - name: DB_USER
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: DB_USER
            - name: DB_NAME
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: DB_NAME
            - name: DB_SCHEMA
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: DB_SCHEMA
            - name: LISTEN_ADDR
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: LISTEN_ADDR
            - name: CONTROL
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: CONTROL
            - name: MARKET_BOT_ENABLED
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: MARKET_BOT_ENABLED
            - name: MARKET_BOT_BUY_INTERVAL
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: MARKET_BOT_BUY_INTERVAL
            - name: MARKET_BOT_LIST_INTERVAL
              valueFrom:
                configMapKeyRef:
                  name: dune-admin-config
                  key: MARKET_BOT_LIST_INTERVAL
            - name: DB_PASS
              valueFrom:
                secretKeyRef:
                  name: dune-admin-secrets
                  key: DB_PASS
            - name: BROKER_USER
              valueFrom:
                secretKeyRef:
                  name: dune-admin-secrets
                  key: BROKER_USER
            - name: BROKER_PASS
              valueFrom:
                secretKeyRef:
                  name: dune-admin-secrets
                  key: BROKER_PASS
            - name: BROKER_JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: dune-admin-secrets
                  key: BROKER_JWT_SECRET
            - name: DUNE_ADMIN_SESSIONS_DB
              value: /data/sessions.db
          volumeMounts:
            - name: app-rw
              mountPath: /app
            - name: market-bot-cache
              mountPath: /data
            - name: config-rw
              mountPath: /root/.dune-admin
          readinessProbe:
            httpGet:
              path: /api/v1/status
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /api/v1/status
              port: http
            initialDelaySeconds: 15
            periodSeconds: 30
      volumes:
        - name: app-rw
          persistentVolumeClaim:
            claimName: dune-admin-app
        - name: market-bot-cache
          persistentVolumeClaim:
            claimName: market-bot-cache
        - name: config-source
          configMap:
            name: dune-admin-config
            items:
              - key: config.yaml
                path: config.yaml
        - name: config-rw
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: dune-admin
  namespace: dune-admin
spec:
  selector:
    app: dune-admin
  ports:
    - name: http
      port: 8080
      targetPort: http
  type: NodePort
`)

	if outPath == "-" {
		fmt.Print(out.String())
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(out.String()), 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", outPath, err)
	}
	return nil
}
