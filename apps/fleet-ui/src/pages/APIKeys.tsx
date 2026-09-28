import { FormEvent, useEffect, useState } from "react";
import { api } from "../api";

type Key = {
  id: string;
  name: string;
  key_prefix: string;
  created_at: string;
  revoked_at?: string;
};

export default function APIKeys() {
  const [keys, setKeys] = useState<Key[]>([]);
  const [name, setName] = useState("");
  const [created, setCreated] = useState("");

  async function refresh() {
    const res = await api<{ keys: Key[] }>("/api/v1/api-keys");
    setKeys(res.keys || []);
  }

  useEffect(() => { refresh().catch(console.error); }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    const res = await api<{ key: string }>("/api/v1/api-keys", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    setCreated(res.key);
    setName("");
    await refresh();
  }

  async function revoke(id: string) {
    await api(`/api/v1/api-keys/${id}`, { method: "DELETE" });
    await refresh();
  }

  return (
    <>
      <div className="top">
        <div>
          <h1>API Keys</h1>
          <p className="muted">Keys for api.teslant.ru (OpenAI-compatible)</p>
        </div>
      </div>

      <form className="panel" onSubmit={create} style={{ marginBottom: "1rem" }}>
        <div className="row">
          <div className="field" style={{ flex: 1, marginBottom: 0 }}>
            <label>Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} required placeholder="app-backend" />
          </div>
          <button className="primary" type="submit" style={{ alignSelf: "end" }}>Create</button>
        </div>
        {created && (
          <div style={{ marginTop: "1rem" }}>
            <p className="muted">Copy now — shown once:</p>
            <div className="pre">{created}</div>
          </div>
        )}
      </form>

      <div className="panel">
        <table className="table">
          <thead><tr><th>Name</th><th>Prefix</th><th>Created</th><th></th></tr></thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.id}>
                <td>{k.name}{k.revoked_at ? " (revoked)" : ""}</td>
                <td className="mono">{k.key_prefix}…</td>
                <td className="muted">{new Date(k.created_at).toLocaleString()}</td>
                <td>{!k.revoked_at && <button className="danger" onClick={() => revoke(k.id)}>Revoke</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
