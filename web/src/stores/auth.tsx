import { createContext, createResource, useContext, type ParentComponent, type Resource } from 'solid-js';
import { fetchMe, type MeResponse } from '../api/auth';

interface AuthContextValue {
  auth: Resource<MeResponse | null>;
  refetch: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export const AuthProvider: ParentComponent = (props) => {
  const [auth, { refetch }] = createResource<MeResponse | null>(fetchMe);
  return (
    <AuthContext.Provider value={{ auth, refetch }}>
      {props.children}
    </AuthContext.Provider>
  );
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
