import React, { useState } from 'react';
import { isAuthenticated } from './api/client';
import LoginScreen from './components/LoginScreen';
import Layout from './components/Layout';

export default function App() {
  const [authed, setAuthed] = useState(isAuthenticated());

  if (!authed) {
    return <LoginScreen onLogin={() => setAuthed(true)} />;
  }

  return <Layout />;
}
