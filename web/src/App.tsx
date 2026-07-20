import { useState, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from './Layout';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import Topology from './pages/Topology';
import NodesPage from './pages/Nodes';
import Enrollment from './pages/Enrollment';
import Policies from './pages/Policies';
import Updates from './pages/Updates';
import Settings from './pages/Settings';
import { api } from './api';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const [authed, setAuthed] = useState<boolean | null>(null);

  useEffect(() => {
    let mounted = true;
    api.me()
      .then(() => { if (mounted) setAuthed(true); })
      .catch(() => { if (mounted) setAuthed(false); });

    const onUnauthorized = () => { if (mounted) setAuthed(false); };
    window.addEventListener('pw:unauthorized', onUnauthorized);
    return () => window.removeEventListener('pw:unauthorized', onUnauthorized);
  }, []);

  if (authed === null) {
    return (
      <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#0f172a', color: '#94a3b8' }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ width: 32, height: 32, border: '3px solid #334155', borderTopColor: '#38bdf8', borderRadius: '50%', animation: 'spin 0.8s linear infinite', margin: '0 auto 12px' }} />
          <span style={{ fontSize: 14 }}>验证身份中...</span>
        </div>
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    );
  }

  if (!authed) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/*" element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/topology" element={<Topology />} />
                <Route path="/nodes" element={<NodesPage />} />
                <Route path="/enrollment" element={<Enrollment />} />
                <Route path="/policies" element={<Policies />} />
                <Route path="/updates" element={<Updates />} />
                <Route path="/settings" element={<Settings />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        } />
      </Routes>
    </BrowserRouter>
  );
}
