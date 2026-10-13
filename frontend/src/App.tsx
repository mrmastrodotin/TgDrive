import React, { useState, useEffect } from 'react';
import Dashboard from './Dashboard';
import LoginScreen from './components/LoginScreen';
import { isAuthenticated } from './api/client';

function App() {
  const [authed, setAuthed] = useState(isAuthenticated());

  // Check auth periodically or on window focus
  useEffect(() => {
    const checkAuth = () => setAuthed(isAuthenticated());
    window.addEventListener('focus', checkAuth);
    return () => window.removeEventListener('focus', checkAuth);
  }, []);

  if (!authed) {
    return <LoginScreen onLogin={() => setAuthed(true)} />;
  }

  return (
    <Dashboard />
  );
}

export default App;
