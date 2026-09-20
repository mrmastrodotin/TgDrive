import React, { useState } from 'react';
import { login } from '../api/client';

interface LoginScreenProps {
  onLogin: () => void;
}

export default function LoginScreen({ onLogin }: LoginScreenProps) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      await login(password);
      onLogin();
    } catch {
      setError('Wrong password');
      setLoading(false);
    }
  };

  return (
    <div className="login-screen">
      <div className="login-card">
        <div className="login-icon">⚡</div>
        <h1 className="login-title">TgDrive</h1>
        <p className="login-subtitle">Encrypted Cloud Storage</p>

        <form onSubmit={handleSubmit} className="login-form">
          <div className="input-group">
            <input
              id="login-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter password"
              className="login-input"
              autoFocus
              autoComplete="current-password"
            />
          </div>

          {error && <div className="login-error">{error}</div>}

          <button
            id="login-submit"
            type="submit"
            className="login-button"
            disabled={loading || !password}
          >
            {loading ? 'Unlocking...' : 'Unlock Drive'}
          </button>
        </form>

        <div className="login-badge">
          <span className="badge-dot"></span>
          AES-256-GCM Encrypted
        </div>
      </div>
    </div>
  );
}
