import React from 'react';
import { Presentation, FileText, ShieldCheck, Download, ExternalLink } from 'lucide-react';
import { FileItem, formatFileSize } from '../../types';
import { getStreamURL } from '../../api/client';

export interface PresentationPreviewProps {
  file: FileItem;
  isOfficeDoc?: boolean;
  onDownload: () => void;
}

export const PresentationPreview: React.FC<PresentationPreviewProps> = ({
  file,
  isOfficeDoc = false,
  onDownload,
}) => {
  const ext = (file.name.split('.').pop() || '').toUpperCase();

  if (isOfficeDoc) {
    return (
      <div className="preview-presentation-viewport">
        <div className="deck-studio-card office-card">
          <div className="deck-screen-frame">
            <div className="deck-slide-mockup">
              <div className="deck-slide-header-bar">
                <span className="deck-badge-pill">Office Document</span>
                <span className="deck-slide-number">{ext}</span>
              </div>
              <div className="deck-slide-body">
                <FileText size={64} className="deck-big-icon text-primary" />
                <h3 className="deck-file-title">{file.name}</h3>
                <p className="deck-file-desc">
                  {formatFileSize(file.size)} • {file.chunk_count} encrypted chunk(s)
                </p>
              </div>
            </div>
          </div>

          <div className="deck-actions-panel">
            <div className="deck-info-box">
              <ShieldCheck size={20} className="text-primary" />
              <div>
                <h5 className="deck-info-title">Secure Office Document</h5>
                <p className="deck-info-sub">
                  Encrypted using AES-256-GCM authenticated storage encryption and distributed across user Telegram bot workers.
                </p>
              </div>
            </div>

            <div className="deck-buttons-row">
              <button className="btn-primary-glow btn-deck-download" onClick={onDownload}>
                <Download size={18} />
                <span>Download & Decrypt Document</span>
              </button>
              <a
                href={getStreamURL(file.id)}
                target="_blank"
                rel="noreferrer"
                className="btn-secondary-glass"
              >
                <ExternalLink size={16} />
                <span>Direct Stream</span>
              </a>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="preview-presentation-viewport">
      <div className="deck-studio-card">
        <div className="deck-screen-frame">
          <div className="deck-slide-mockup">
            <div className="deck-slide-header-bar">
              <span className="deck-badge-pill">PowerPoint Presentation</span>
              <span className="deck-slide-number">Slide Deck</span>
            </div>
            <div className="deck-slide-body">
              <Presentation size={64} className="deck-big-icon" />
              <h3 className="deck-file-title">{file.name}</h3>
              <p className="deck-file-desc">
                Encrypted in {file.chunk_count} AES-256 segment(s) • {formatFileSize(file.size)}
              </p>
            </div>
          </div>
        </div>

        <div className="deck-actions-panel">
          <div className="deck-info-box">
            <ShieldCheck size={20} className="text-primary" />
            <div>
              <h5 className="deck-info-title">Hardware Encrypted Slide Deck</h5>
              <p className="deck-info-sub">
                All slides, speaker notes, and embedded media are securely stored in chunks on Telegram.
              </p>
            </div>
          </div>

          <div className="deck-buttons-row">
            <button className="btn-primary-glow btn-deck-download" onClick={onDownload}>
              <Download size={18} />
              <span>Download & Decrypt Deck</span>
            </button>
            <a
              href={getStreamURL(file.id)}
              target="_blank"
              rel="noreferrer"
              className="btn-secondary-glass"
            >
              <ExternalLink size={16} />
              <span>Direct Stream</span>
            </a>
          </div>
        </div>
      </div>
    </div>
  );
};
