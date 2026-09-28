import { Navigate, NavLink, Route, Routes, useNavigate } from "react-router-dom";
import { getToken, setToken } from "./api";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import Nodes from "./pages/Nodes";
import Invite from "./pages/Invite";
import Models from "./pages/Models";
import APIKeys from "./pages/APIKeys";

function Shell({ children }: { children: React.ReactNode }) {
  const nav = useNavigate();
  return (
    <div className="shell">
      <aside className="nav">
        <div className="brand">Pirate <span>Fleet</span></div>
        <NavLink to="/" end>Overview</NavLink>
        <NavLink to="/nodes">Nodes</NavLink>
        <NavLink to="/models">Models</NavLink>
        <NavLink to="/keys">API Keys</NavLink>
        <button style={{ marginTop: "2rem", width: "100%" }} onClick={() => { setToken(null); nav("/login"); }}>
          Sign out
        </button>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}

function Private({ children }: { children: React.ReactNode }) {
  if (!getToken()) return <Navigate to="/login" replace />;
  return <Shell>{children}</Shell>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/" element={<Private><Dashboard /></Private>} />
      <Route path="/nodes" element={<Private><Nodes /></Private>} />
      <Route path="/nodes/invite/:id" element={<Private><Invite /></Private>} />
      <Route path="/models" element={<Private><Models /></Private>} />
      <Route path="/keys" element={<Private><APIKeys /></Private>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
