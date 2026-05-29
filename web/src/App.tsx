import { useState, useEffect } from "react";
import { Show, SignInButton, UserButton, useAuth } from "@clerk/react";
import { Button, Chip, InputGroup, Modal, TextField, Toast, Tabs } from "@heroui/react";
import { useLocation, useNavigate } from "react-router-dom";
import { useStatus } from "./hooks/useStatus";
import BattlegroupTab from "./tabs/BattlegroupTab";
import PlayersTab from "./tabs/PlayersTab";
import DatabaseTab from "./tabs/DatabaseTab";
import LogsTab from "./tabs/LogsTab";
import BlueprintsTab from "./tabs/BlueprintsTab";
import BasesTab from "./tabs/BasesTab";
import StorageTab from "./tabs/StorageTab";
import ServerSettingsTab from "./tabs/ServerSettingsTab";
import MarketTab from "./tabs/MarketTab";
import { Icon } from "./dune-ui";

const TAB_IDS = [
  "battlegroup",
  "players",
  "database",
  "logs",
  "blueprints",
  "bases",
  "storage",
  "server",
  "market",
] as const;
type TabId = (typeof TAB_IDS)[number];
const DEFAULT_TAB: TabId = "battlegroup";

function currentTabFromPath(pathname: string): TabId {
  const seg = pathname.replace(/^\//, "").split("/")[0];
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB;
}

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

function AppWithAuth() {
  const { isSignedIn } = useAuth();
  return <AppCore isSignedIn={!!isSignedIn} />;
}

export default function App() {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />;
}

function parseVer(v: string): [number, number, number] {
  const [a, b, c] = v.replace(/^v/, "").split(".").map(Number);
  return [a || 0, b || 0, c || 0];
}

function isNewer(latest: string, current: string): boolean {
  if (current === "dev") return false;
  const [la, lb, lc] = parseVer(latest);
  const [ca, cb, cc] = parseVer(current);
  if (la !== ca) return la > ca;
  if (lb !== cb) return lb > cb;
  return lc > cc;
}

function AppCore({ isSignedIn }: { isSignedIn: boolean }) {
  const status = useStatus();
  const location = useLocation();
  const navigate = useNavigate();
  const [showBackendConfig, setShowBackendConfig] = useState(false);
  const [backendUrl, setBackendUrl] = useState(() => localStorage.getItem("dune_admin_backend") || "");
  const [latestVersion, setLatestVersion] = useState<string | null>(null);

  useEffect(() => {
    const seg = location.pathname.replace(/^\//, "").split("/")[0];
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${DEFAULT_TAB}`, { replace: true });
    }
  }, [location.pathname, navigate]);

  const currentTab = currentTabFromPath(location.pathname);

  useEffect(() => {
    fetch("https://api.github.com/repos/Icehunter/dune-admin/releases/latest")
      .then((r) => r.json())
      .then((d) => setLatestVersion(d.tag_name || null))
      .catch(() => {});
  }, []);

  const saveAndReload = () => {
    localStorage.setItem("dune_admin_backend", backendUrl.trim());
    window.location.reload();
  };

  return (
    <div className="h-screen flex flex-col overflow-hidden bg-background">
      <Toast.Provider />

      {/* Header */}
      <header
        className="flex items-center justify-between px-6 py-3 border-b border-[#4e3411] bg-surface shrink-0"
        style={{ background: "linear-gradient(180deg, #241a0e 0%, #1a1610 100%)" }}
      >
        <div className="flex items-center gap-3">
          <span className="text-xl font-bold uppercase tracking-[0.2em] text-accent">DUNE ADMIN</span>
          {status?.control && status.control !== "none" && <span className="text-xs text-muted">{status.control}</span>}
          {status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {status?.db_host && status.control !== "kubectl" && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && <span className="text-xs text-muted">v{status.version}</span>}
          {latestVersion && status?.version && isNewer(latestVersion, status.version) && (
            <a
              href="https://github.com/Icehunter/dune-admin/releases/latest"
              target="_blank"
              rel="noreferrer"
              className="no-underline"
            >
              <Chip size="sm" color="warning" variant="soft">
                ↑ {latestVersion}
              </Chip>
            </a>
          )}
        </div>

        <div className="flex items-center gap-3">
          {status?.executor === "ssh" && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />
          {status?.pod_ns && <span className="text-xs text-muted">ns: {status.pod_ns}</span>}

          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label="Configure backend"
            onPress={() => setShowBackendConfig((v) => !v)}
            className={showBackendConfig ? "text-accent" : ""}
          >
            <Icon name="settings" />
          </Button>

          {hasClerk && (
            <>
              <Show when="signed-out">
                <SignInButton>
                  <Button size="sm" variant="outline">
                    Sign In
                  </Button>
                </SignInButton>
              </Show>
              <Show when="signed-in">
                <UserButton />
              </Show>
            </>
          )}
        </div>
      </header>

      {/* Settings modal */}
      <Modal>
        <Modal.Backdrop isOpen={showBackendConfig} onOpenChange={(v) => !v && setShowBackendConfig(false)}>
          <Modal.Container size="sm" scroll="outside">
            <Modal.Dialog>
              <Modal.CloseTrigger />
              <Modal.Header>
                <Modal.Heading className="text-accent">Settings</Modal.Heading>
              </Modal.Header>

              <Modal.Body className="flex flex-col gap-4">
                {/* About */}
                {status && (
                  <>
                    <div className="flex flex-col gap-2">
                      <span className="text-xs font-medium text-muted uppercase tracking-wider">About</span>
                      <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
                        {status.version && (
                          <>
                            <span className="text-muted">Version</span>
                            <span className="font-mono text-foreground">v{status.version}</span>
                          </>
                        )}
                        {status.commit && status.commit !== "unknown" && (
                          <>
                            <span className="text-muted">Commit</span>
                            <span className="font-mono text-foreground">{status.commit}</span>
                          </>
                        )}
                        {status.control && status.control !== "none" && (
                          <>
                            <span className="text-muted">Control</span>
                            <span className="font-mono text-foreground">{status.control}</span>
                          </>
                        )}
                        {status.build_time && status.build_time !== "unknown" && (
                          <>
                            <span className="text-muted">Built</span>
                            <span className="font-mono text-foreground">
                              {new Date(status.build_time).toLocaleString()}
                            </span>
                          </>
                        )}
                      </div>
                    </div>
                    <div className="border-t border-border" />
                  </>
                )}

                {/* Backend URL override */}
                <div className="flex flex-col gap-3">
                  <div className="flex flex-col gap-1">
                    <span className="text-xs font-medium text-muted uppercase tracking-wider">Backend URL Override</span>
                    <p className="text-xs text-muted">
                      Only needed when the UI is served from a different host than the backend
                      (e.g. SSH tunnel or CDN deploy). Leave blank for the default single-binary setup.
                    </p>
                  </div>

                  <p className="text-xs text-muted">
                    Current:{" "}
                    <span className="font-mono text-foreground">
                      {localStorage.getItem("dune_admin_backend") || "http://localhost:8080"}
                    </span>
                  </p>

                  <TextField aria-label="Backend URL override">
                    <InputGroup className="w-full">
                      <InputGroup.Prefix>URL</InputGroup.Prefix>
                      <InputGroup.Input
                        value={backendUrl}
                        onChange={(e) => setBackendUrl(e.target.value)}
                        placeholder="http://host:port"
                        className="font-mono"
                        onKeyDown={(e) => {
                          if (e.key === "Enter") saveAndReload();
                        }}
                      />
                    </InputGroup>
                  </TextField>
                </div>
              </Modal.Body>

              <Modal.Footer>
                <Button size="sm" variant="outline" onPress={() => setShowBackendConfig(false)}>
                  Cancel
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onPress={() => {
                    localStorage.removeItem("dune_admin_backend");
                    window.location.reload();
                  }}
                >
                  Reset URL
                </Button>
                <Button size="sm" onPress={saveAndReload}>
                  Save &amp; Reload
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Tabs */}
      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        <Tabs
          selectedKey={currentTab}
          onSelectionChange={(k) => navigate(`/${k}`)}
          className="flex-1 flex flex-col overflow-hidden min-h-0"
        >
          <Tabs.ListContainer className="px-4 pt-2 shrink-0">
            <Tabs.List aria-label="Admin sections" className="gap-1">
              <Tabs.Tab id="battlegroup">
                Battlegroup
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="players">
                Players
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="database">
                Database
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="logs">
                Logs
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="blueprints">
                Blueprints
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="bases">
                Bases
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="storage">
                Storage
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="server">
                Server
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="market">
                Market
                <Tabs.Indicator />
              </Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
          <Tabs.Panel id="battlegroup" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BattlegroupTab />
          </Tabs.Panel>
          <Tabs.Panel id="players" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <PlayersTab />
          </Tabs.Panel>
          <Tabs.Panel id="database" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <DatabaseTab />
          </Tabs.Panel>
          <Tabs.Panel id="logs" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <LogsTab />
          </Tabs.Panel>
          <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BlueprintsTab isSignedIn={isSignedIn} />
          </Tabs.Panel>
          <Tabs.Panel id="bases" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BasesTab isSignedIn={isSignedIn} />
          </Tabs.Panel>
          <Tabs.Panel id="storage" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <StorageTab />
          </Tabs.Panel>
          <Tabs.Panel id="server" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <ServerSettingsTab />
          </Tabs.Panel>
          <Tabs.Panel id="market" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <MarketTab />
          </Tabs.Panel>
        </Tabs>
      </div>
    </div>
  );
}

function ConnectionBadge({ label, connected }: { label: string; connected: boolean }) {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      <div className={`w-2 h-2 rounded-full ${connected ? "bg-success" : "bg-muted/40"}`} />
      <span className={connected ? "text-foreground" : "text-muted"}>{label}</span>
    </div>
  );
}
