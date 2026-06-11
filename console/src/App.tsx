import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth";
import { Loading } from "./components/ui";
import Login from "./pages/Login";
import Signup from "./pages/Signup";
import Agents from "./pages/Agents";
import AgentDashboard from "./pages/AgentDashboard";
import ReviewQueue from "./pages/ReviewQueue";
import Memories from "./pages/Memories";
import Timeline from "./pages/Timeline";
import TimeMachine from "./pages/TimeMachine";
import Contradictions from "./pages/Contradictions";
import Keys from "./pages/Keys";
import Canon from "./pages/Canon";
import Audit from "./pages/Audit";
import Billing from "./pages/Billing";
import Settings from "./pages/Settings";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) return <Loading />;
  if (!me) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function PublicOnly({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) return <Loading />;
  if (me) return <Navigate to="/agents" replace />;
  return <>{children}</>;
}

const protect = (el: React.ReactNode) => <RequireAuth>{el}</RequireAuth>;

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<PublicOnly><Login /></PublicOnly>} />
      <Route path="/signup" element={<PublicOnly><Signup /></PublicOnly>} />
      <Route path="/" element={<Navigate to="/agents" replace />} />
      <Route path="/agents" element={protect(<Agents />)} />
      <Route path="/agents/:agentId" element={protect(<AgentDashboard />)} />
      <Route path="/agents/:agentId/memories" element={protect(<Memories />)} />
      <Route path="/agents/:agentId/review" element={protect(<ReviewQueue />)} />
      <Route path="/agents/:agentId/contradictions" element={protect(<Contradictions />)} />
      <Route path="/agents/:agentId/timemachine" element={protect(<TimeMachine />)} />
      <Route path="/memories/:memoryId" element={protect(<Timeline />)} />
      <Route path="/canon" element={protect(<Canon />)} />
      <Route path="/keys" element={protect(<Keys />)} />
      <Route path="/audit" element={protect(<Audit />)} />
      <Route path="/billing" element={protect(<Billing />)} />
      <Route path="/settings" element={protect(<Settings />)} />
      <Route path="*" element={<Navigate to="/agents" replace />} />
    </Routes>
  );
}
