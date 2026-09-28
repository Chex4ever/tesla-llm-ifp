import { FormEvent, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api";

type Node = {
  id: string;
  name: string;
  status: string;
  tags?: string[];
  gpu_name?: string;
  vram_mb?: number;
  runtime_healthy?: boolean;
  drained?: boolean;
  models_ready?: string[];
  os?: string;
  agent_version?: string;
  last_heartbeat?: string;
};

export default function Nodes() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [name, setName] = useState("");
  const [tags, setTags] = useState("");
  const [error, setError] = useState("");
  const nav = useNavigate();

  async function refresh() {
    const res = await api<{ nodes: Node[] }>("/api/v1/nodes");
    setNodes(res.nodes || []);
  }

  useEffect(() => {
    refresh().catch(console.error);
    const t = setInterval(() => refresh().catch(() => {}), 10000);
    return () => clearInterval(t);
  }, []);

  async function createInvite(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const res = await api<any>("/api/v1/nodes/invites", {
        method: "POST",
        body: JSON.stringify({
          name,
          tags: tags.split(",").map((t) => t.trim()).filter(Boolean),
          ttl_minutes: 1440,
        }),
      });
      sessionStorage.setItem(`invite:${res.invite_id}`, JSON.stringify(res));
      nav(`/nodes/invite/${res.invite_id}`);
    } catch (err: any) {
      setError(err.message || "Failed");
    }
  }

  async function drain(id: string, on: boolean) {
    await api(`/api/v1/nodes/${id}/${on ? "drain" : "undrain"}`, { method: "POST" });
    await refresh();
  }

  async function remove(id: string) {
    if (!confirm("Delete node?")) return;
    await api(`/api/v1/nodes/${id}`, { method: "DELETE" });
    await refresh();
  }

  return (
    <>
      <div className="top">
        <div>
          <h1>Nodes</h1>
          <p className="muted">Onboard Windows PCs and manage workers</p>
        </div>
      </div>

      <form className="panel" onSubmit={createInvite} style={{ marginBottom: "1.25rem" }}>
        <h3 style={{ marginTop: 0 }}>Add PC</h3>
        <div className="row">
          <div className="field" style={{ flex: 1, minWidth: 180, marginBottom: 0 }}>
            <label>Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="office-gpu-01" required />
          </div>
          <div className="field" style={{ flex: 1, minWidth: 180, marginBottom: 0 }}>
            <label>Tags (comma)</label>
            <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="windows, fat-pipe" />
          </div>
          <button className="primary" type="submit" style={{ alignSelf: "end" }}>Create invite</button>
        </div>
        {error && <p className="error">{error}</p>}
      </form>

      <div className="panel">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Status</th>
              <th>GPU</th>
              <th>Models</th>
              <th>OS / Agent</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.id}>
                <td>
                  <strong>{n.name}</strong>
                  <div className="muted mono">{n.id.slice(0, 8)}</div>
                </td>
                <td>
                  <span className={`badge ${n.status}`}>{n.status}</span>
                  {n.drained && <span className="badge stale" style={{ marginLeft: 6 }}>drained</span>}
                  {n.runtime_healthy === false && n.status === "online" && (
                    <span className="badge stale" style={{ marginLeft: 6 }}>runtime</span>
                  )}
                </td>
                <td>{n.gpu_name || "—"}{n.vram_mb ? ` (${n.vram_mb} MB)` : ""}</td>
                <td className="mono">{(n.models_ready || []).join(", ") || "—"}</td>
                <td className="muted">{n.os || "—"} · {n.agent_version || "—"}</td>
                <td>
                  <div className="row">
                    <button onClick={() => drain(n.id, !n.drained)}>{n.drained ? "Undrain" : "Drain"}</button>
                    <button className="danger" onClick={() => remove(n.id)}>Delete</button>
                  </div>
                </td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr><td colSpan={6} className="muted">No nodes yet. Create an invite above.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      <p className="muted" style={{ marginTop: "1rem" }}>
        After creating an invite, open the instruction page (auto-redirect) or find it under{" "}
        <Link to="/nodes">Nodes</Link>.
      </p>
    </>
  );
}
