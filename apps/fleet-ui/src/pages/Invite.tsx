import { useParams } from "react-router-dom";
import { useMemo, useState } from "react";

export default function Invite() {
  const { id } = useParams();
  const data = useMemo(() => {
    try {
      return JSON.parse(sessionStorage.getItem(`invite:${id}`) || "null");
    } catch {
      return null;
    }
  }, [id]);
  const [copied, setCopied] = useState("");

  if (!data) {
    return (
      <div className="panel">
        <h1>Invite</h1>
        <p className="muted">Invite details are only shown once after creation. Create a new invite from Nodes.</p>
        <p className="mono">invite id: {id}</p>
      </div>
    );
  }

  async function copy(text: string, label: string) {
    await navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(""), 1500);
  }

  return (
    <>
      <div className="top">
        <div>
          <h1>Onboard: {data.name || "PC"}</h1>
          <p className="muted">Centralized install instructions — share with the machine owner</p>
        </div>
      </div>

      <div className="panel" style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>1. Download Windows agent</h3>
        <p className="muted">Installer / binary from control plane:</p>
        <p className="mono"><a href={data.download_windows}>{data.download_windows}</a></p>
        <button onClick={() => copy(data.download_windows, "url")}>Copy URL</button>
        {copied === "url" && <span className="muted"> copied</span>}
      </div>

      <div className="panel" style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>2. Invite token</h3>
        <div className="pre">{data.token}</div>
        <button style={{ marginTop: "0.75rem" }} onClick={() => copy(data.token, "token")}>Copy token</button>
        {copied === "token" && <span className="muted"> copied</span>}
        <p className="muted">Expires: {new Date(data.expires_at).toLocaleString()}</p>
      </div>

      <div className="panel" style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginTop: 0 }}>3. Windows instructions</h3>
        <div className="pre">{data.instructions?.windows}</div>
        <p className="muted" style={{ marginTop: "0.75rem" }}>
          Prerequisites: install <a href="https://ollama.com/download" target="_blank" rel="noreferrer">Ollama</a>,
          NVIDIA drivers if GPU. Optional: Tailscale client for Headscale overlay ({data.headscale_login_server}).
        </p>
      </div>

      <div className="panel">
        <h3 style={{ marginTop: 0 }}>Linux (later)</h3>
        <div className="pre">{data.instructions?.linux}</div>
      </div>
    </>
  );
}
