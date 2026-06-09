import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { Auth, AuthConfig, Me } from "./api";

interface AuthState {
  me: Me | null;
  config: AuthConfig | null;
  loading: boolean;
  refresh: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => Promise<void>;
  switchOrg: (tenantId: string) => Promise<void>;
}

const Ctx = createContext<AuthState>(null as unknown as AuthState);
export const useAuth = () => useContext(Ctx);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [config, setConfig] = useState<AuthConfig | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setMe(await Auth.me());
    } catch {
      setMe(null);
    }
  }, []);

  useEffect(() => {
    (async () => {
      try {
        setConfig(await Auth.config());
      } catch {
        setConfig({ password: true, google: false, github: false, workos: false });
      }
      await refresh();
      setLoading(false);
    })();
  }, [refresh]);

  useEffect(() => {
    const onUnauth = () => setMe(null);
    window.addEventListener("engram:unauthorized", onUnauth);
    return () => window.removeEventListener("engram:unauthorized", onUnauth);
  }, []);

  const login = async (email: string, password: string) => {
    await Auth.login(email, password);
    await refresh();
  };
  const register = async (email: string, password: string, name: string) => {
    await Auth.register(email, password, name);
    await refresh();
  };
  const logout = async () => {
    await Auth.logout();
    setMe(null);
  };
  const switchOrg = async (tenantId: string) => {
    await Auth.switchOrg(tenantId);
    await refresh();
  };

  return (
    <Ctx.Provider value={{ me, config, loading, refresh, login, register, logout, switchOrg }}>
      {children}
    </Ctx.Provider>
  );
}
