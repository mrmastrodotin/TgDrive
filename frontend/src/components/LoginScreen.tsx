import React, { useState } from 'react';
import { ShieldCheck, Lock, Eye, EyeOff, Sparkles, ArrowRight } from 'lucide-react';
import { login } from '../api/client';

interface LoginScreenProps {
  onLogin: () => void;
}

export default function LoginScreen({ onLogin }: LoginScreenProps) {
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;
    setError('');
    setLoading(true);

    try {
      await login(password);
      onLogin();
    } catch {
      setError('Access denied: Invalid master password');
      setLoading(false);
    }
  };

  return (
    <div className="login-container">
      {/* Dynamic Background Glows */}
      <div className="glow-sphere glow-1"></div>
      <div className="glow-sphere glow-2"></div>
      <div className="cyber-grid"></div>

      <div className="login-card-glass">
        <div className="vault-badge-header">
          <div className="vault-icon-wrapper">
            <ShieldCheck className="vault-icon-svg" />
            <div className="vault-icon-pulse"></div>
          </div>
          <h1 className="vault-title">TgDrive</h1>
          <p className="vault-tagline">Encrypted Personal Cloud Vault</p>
        </div>

        <form onSubmit={handleSubmit} className="vault-form">
          <div className="vault-input-container">
            <div className="input-icon-prefix">
              <Lock size={18} />
            </div>
            <input
              id="login-password"
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter master password..."
              className="vault-input-field"
              autoFocus
              autoComplete="current-password"
            />
            <button
              type="button"
              className="password-toggle-btn"
              onClick={() => setShowPassword(!showPassword)}
              tabIndex={-1}
              aria-label="Toggle password visibility"
            >
              {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </div>

          {error && (
            <div className="vault-error-alert animate-shake">
              <span>{error}</span>
            </div>
          )}

          <button
            id="login-submit"
            type="submit"
            className="vault-unlock-btn"
            disabled={loading || !password.trim()}
          >
            {loading ? (
              <span className="btn-loading-flex">
                <span className="spinner-border"></span>
                <span>Unlocking Vault...</span>
              </span>
            ) : (
              <span className="btn-content-flex">
                <span>Unlock TgDrive</span>
                <ArrowRight size={18} className="btn-arrow" />
              </span>
            )}
          </button>
        </form>

        <div className="vault-footer-security">
          <div className="security-pill">
            <span className="live-dot"></span>
            <span>AES-256-GCM Military Grade</span>
          </div>
          <div className="security-pill">
            <Sparkles size={13} className="text-accent" />
            <span>Telegram Cloud Backbone</span>
          </div>
        </div>
      </div>
    </div>
  );
}
