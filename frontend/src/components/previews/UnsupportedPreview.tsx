import React from 'react';
import { Layers, Download } from 'lucide-react';
import { FileItem, formatFileSize } from '../../types';

export interface UnsupportedPreviewProps {
  file: FileItem;
  onDownload: () => void;
}

export const UnsupportedPreview: React.FC<UnsupportedPreviewProps> = ({
  file,
  onDownload,
}) => {
  return (
    <div className="preview-unsupported-viewport">
      <div className="unsupported-card">
        <Layers size={54} className="unsupported-icon" />
        <h4 className="unsupported-title">{file.name}</h4>
        <p className="unsupported-sub">
          Binary format • {formatFileSize(file.size)} • {file.chunk_count} chunk(s)
        </p>
        <div className="unsupported-telemetry">
          <div className="tele-item">
            <span className="tele-lbl">MIME</span>
            <span className="tele-val">{file.mime_type || 'application/octet-stream'}</span>
          </div>
          <div className="tele-item">
            <span className="tele-lbl">CIPHER</span>
            <span className="tele-val">AES-256-GCM</span>
          </div>
          <div className="tele-item">
            <span className="tele-lbl">SALT</span>
            <span className="tele-val font-mono">
              {file.encryption_salt ? `${file.encryption_salt.substring(0, 16)}...` : 'AES-GCM-STANDARD'}
            </span>
          </div>
        </div>
        <button className="btn-primary-glow" onClick={onDownload}>
          <Download size={16} />
          <span>Download & Decrypt File</span>
        </button>
      </div>
    </div>
  );
};
