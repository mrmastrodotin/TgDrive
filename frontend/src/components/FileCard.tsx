import React from 'react';
import { FileItem, formatFileSize, getFileIcon, getPreviewType } from '../types';
import { downloadFile, getStreamURL, getPreviewURL } from '../api/client';

interface FileCardProps {
  file: FileItem;
  onDelete: (id: string) => void;
  onPreview: (file: FileItem) => void;
}

export default function FileCard({ file, onDelete, onPreview }: FileCardProps) {
  const previewType = getPreviewType(file.mime_type, file.name);
  const icon = getFileIcon(file.mime_type);
  const date = new Date(file.created_at).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric'
  });

  const handleDownload = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await downloadFile(file.id, file.name);
    } catch (err) {
      console.error('Download failed:', err);
    }
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (confirm(`Delete "${file.name}"?`)) {
      onDelete(file.id);
    }
  };

  return (
    <div
      className="file-card"
      onClick={() => previewType !== 'none' && onPreview(file)}
      role="button"
      tabIndex={0}
    >
      <div className="file-card-header">
        <div className="file-card-icon">{icon}</div>
        <div className="file-card-info">
          <p className="file-card-name" title={file.name}>{file.name}</p>
          <p className="file-card-meta">
            {formatFileSize(file.size)} · {date}
            {file.chunk_count > 1 && ` · ${file.chunk_count} chunks`}
          </p>
        </div>
      </div>

      <div className="file-card-actions">
        {previewType !== 'none' && (
          <button
            className="btn-icon"
            onClick={(e) => { e.stopPropagation(); onPreview(file); }}
            title="Preview"
          >
            👁️
          </button>
        )}
        <button className="btn-icon" onClick={handleDownload} title="Download">
          ⬇️
        </button>
        <button className="btn-icon btn-danger" onClick={handleDelete} title="Delete">
          🗑️
        </button>
      </div>
    </div>
  );
}
