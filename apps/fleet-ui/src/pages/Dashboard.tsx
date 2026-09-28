import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";

export default function Dashboard() {
  const [stats, setStats] = useState({ nodes_total: 0, nodes_online: 0, nodes_pending: 0, models: 0 });

  useEffect(() => {
    api<typeof stats>("/api/v1/stats").then(setStats).catch(console.error);
    const t = setInterval(() => api<typeof stats>("/api/v1/stats").then(setStats).catch(() => {}), 15000);
    return () => clearInterval(t);
  }, []);

  return (
    <>
      <div className="top">
        <div>
          <h1>Overview</h1>
          <p className="muted">Fleet health and quick actions</p>
        </div>
        <Link to="/nodes"><button className="primary">Add PC</button></Link>
      </div>
      <div className="grid">
        <div className="stat"><div className="muted">Online nodes</div><div className="n">{stats.nodes_online}</div></div>
        <div className="stat"><div className="muted">Total nodes</div><div className="n">{stats.nodes_total}</div></div>
        <div className="stat"><div className="muted">Pending invites</div><div className="n">{stats.nodes_pending}</div></div>
        <div className="stat"><div className="muted">Models</div><div className="n">{stats.models}</div></div>
      </div>
      <div className="panel" style={{ marginTop: "1.25rem" }}>
        <h3 style={{ marginTop: 0 }}>Domains</h3>
        <p className="muted">api · fleet · chat · grafana · models · registry · hs — see README for DNS checklist.</p>
      </div>
    </>
  );
}
