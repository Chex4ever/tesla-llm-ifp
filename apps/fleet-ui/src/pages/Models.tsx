import { FormEvent, useEffect, useState } from "react";
import { api } from "../api";

type Model = {
  id: string;
  model_id: string;
  display_name: string;
  format: string;
  ollama_tag?: string;
  min_vram_mb: number;
  size_bytes: number;
};

type Node = { id: string; name: string; status: string };

export default function Models() {
  const [models, setModels] = useState<Model[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [modelId, setModelId] = useState("");
  const [display, setDisplay] = useState("");
  const [tag, setTag] = useState("");
  const [format, setFormat] = useState("ollama");
  const [minVram, setMinVram] = useState(8192);
  const [assignNode, setAssignNode] = useState("");
  const [assignModel, setAssignModel] = useState("");
  const [objectName, setObjectName] = useState("");
  const [uploadUrl, setUploadUrl] = useState("");

  async function refresh() {
    const [m, n] = await Promise.all([
      api<{ models: Model[] }>("/api/v1/models"),
      api<{ nodes: Node[] }>("/api/v1/nodes"),
    ]);
    setModels(m.models || []);
    setNodes(n.nodes || []);
  }

  useEffect(() => { refresh().catch(console.error); }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    await api("/api/v1/models", {
      method: "POST",
      body: JSON.stringify({
        model_id: modelId,
        display_name: display || modelId,
        format,
        ollama_tag: tag || modelId,
        min_vram_mb: minVram,
        minio_object: objectName || undefined,
      }),
    });
    setModelId(""); setDisplay(""); setTag("");
    await refresh();
  }

  async function assign(e: FormEvent) {
    e.preventDefault();
    await api(`/api/v1/nodes/${assignNode}/models/${assignModel}`, { method: "POST" });
    alert("Model assigned — agent will pull on next heartbeat");
  }

  async function getUpload(e: FormEvent) {
    e.preventDefault();
    const res = await api<{ upload_url: string }>("/api/v1/models/upload-url", {
      method: "POST",
      body: JSON.stringify({ object_name: objectName }),
    });
    setUploadUrl(res.upload_url);
  }

  return (
    <>
      <div className="top">
        <div>
          <h1>Models</h1>
          <p className="muted">Registry and node assignment</p>
        </div>
      </div>

      <form className="panel" onSubmit={create} style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>Register model</h3>
        <div className="row">
          <div className="field" style={{ flex: 1 }}><label>Model ID</label><input value={modelId} onChange={(e) => setModelId(e.target.value)} required placeholder="llama3.1:8b" /></div>
          <div className="field" style={{ flex: 1 }}><label>Display name</label><input value={display} onChange={(e) => setDisplay(e.target.value)} /></div>
          <div className="field" style={{ flex: 1 }}><label>Ollama tag</label><input value={tag} onChange={(e) => setTag(e.target.value)} placeholder="llama3.1:8b" /></div>
        </div>
        <div className="row">
          <div className="field" style={{ flex: 1 }}>
            <label>Format</label>
            <select value={format} onChange={(e) => setFormat(e.target.value)}>
              <option value="ollama">ollama</option>
              <option value="gguf">gguf</option>
            </select>
          </div>
          <div className="field" style={{ flex: 1 }}><label>Min VRAM MB</label><input type="number" value={minVram} onChange={(e) => setMinVram(Number(e.target.value))} /></div>
          <div className="field" style={{ flex: 1 }}><label>MinIO object (optional)</label><input value={objectName} onChange={(e) => setObjectName(e.target.value)} placeholder="gguf/model.gguf" /></div>
        </div>
        <button className="primary" type="submit">Register</button>
      </form>

      <form className="panel" onSubmit={getUpload} style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>Presigned upload URL</h3>
        <div className="row">
          <div className="field" style={{ flex: 1, marginBottom: 0 }}>
            <label>Object name</label>
            <input value={objectName} onChange={(e) => setObjectName(e.target.value)} required />
          </div>
          <button type="submit" style={{ alignSelf: "end" }}>Get URL</button>
        </div>
        {uploadUrl && <div className="pre" style={{ marginTop: "0.75rem" }}>{uploadUrl}</div>}
      </form>

      <form className="panel" onSubmit={assign} style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>Assign to node</h3>
        <div className="row">
          <div className="field" style={{ flex: 1 }}>
            <label>Node</label>
            <select value={assignNode} onChange={(e) => setAssignNode(e.target.value)} required>
              <option value="">Select…</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name} ({n.status})</option>)}
            </select>
          </div>
          <div className="field" style={{ flex: 1 }}>
            <label>Model</label>
            <select value={assignModel} onChange={(e) => setAssignModel(e.target.value)} required>
              <option value="">Select…</option>
              {models.map((m) => <option key={m.id} value={m.model_id}>{m.display_name}</option>)}
            </select>
          </div>
          <button className="primary" type="submit" style={{ alignSelf: "end" }}>Assign</button>
        </div>
      </form>

      <div className="panel">
        <table className="table">
          <thead><tr><th>ID</th><th>Name</th><th>Format</th><th>Tag</th><th>Min VRAM</th></tr></thead>
          <tbody>
            {models.map((m) => (
              <tr key={m.id}>
                <td className="mono">{m.model_id}</td>
                <td>{m.display_name}</td>
                <td>{m.format}</td>
                <td className="mono">{m.ollama_tag || "—"}</td>
                <td>{m.min_vram_mb}</td>
              </tr>
            ))}
            {models.length === 0 && <tr><td colSpan={5} className="muted">No models registered</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  );
}
