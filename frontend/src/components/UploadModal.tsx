import React, { useState, useRef, useCallback } from 'react';
import { uploadFile } from '../api/client';

interface UploadModalProps {
  folderId?: string;
  onClose: () => void;
  onUploadComplete: () => void;
}

interface UploadItem {
  file: File;
  progress: number;
  status: 'pending' | 'uploading' | 'done' | 'error';
  error?: string;
}

export default function UploadModal({ folderId, onClose, onUploadComplete }: UploadModalProps) {
  const [items, setItems] = useState<UploadItem[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const addFiles = (files: FileList | File[]) => {
    const newItems: UploadItem[] = Array.from(files).map((file) => ({
      file,
      progress: 0,
      status: 'pending' as const,
    }));
    setItems((prev) => [...prev, ...newItems]);
  };

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback(() => {
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    if (e.dataTransfer.files.length > 0) {
      addFiles(e.dataTransfer.files);
    }
  }, []);

  const startUpload = async () => {
    for (let i = 0; i < items.length; i++) {
      if (items[i].status !== 'pending') continue;

      setItems((prev) => {
        const next = [...prev];
        next[i] = { ...next[i], status: 'uploading' };
        return next;
      });

      try {
        await uploadFile(items[i].file, folderId, (percent) => {
          setItems((prev) => {
            const next = [...prev];
            next[i] = { ...next[i], progress: percent };
            return next;
          });
        });

        setItems((prev) => {
          const next = [...prev];
          next[i] = { ...next[i], status: 'done', progress: 100 };
          return next;
        });
      } catch (err: any) {
        setItems((prev) => {
          const next = [...prev];
          next[i] = { ...next[i], status: 'error', error: err.message };
          return next;
        });
      }
    }
    onUploadComplete();
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
  };

  const hasPending = items.some((i) => i.status === 'pending');
  const allDone = items.length > 0 && items.every((i) => i.status === 'done' || i.status === 'error');

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>Upload Files</h2>
          <button id="upload-close" className="modal-close" onClick={onClose}>✕</button>
        </div>

        <div
          className={`drop-zone ${isDragging ? 'drop-zone-active' : ''}`}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
        >
          <div className="drop-zone-icon">📂</div>
          <p className="drop-zone-text">
            {isDragging ? 'Drop files here' : 'Drag & drop files or click to browse'}
          </p>
          <p className="drop-zone-hint">Files are encrypted before upload</p>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="drop-zone-input"
            onChange={(e) => e.target.files && addFiles(e.target.files)}
          />
        </div>

        {items.length > 0 && (
          <div className="upload-list">
            {items.map((item, idx) => (
              <div key={idx} className="upload-item">
                <div className="upload-item-info">
                  <span className="upload-item-name">{item.file.name}</span>
                  <span className="upload-item-size">{formatSize(item.file.size)}</span>
                </div>
                <div className="upload-item-progress">
                  <div className="progress-bar">
                    <div
                      className={`progress-fill ${item.status === 'error' ? 'progress-error' : ''} ${item.status === 'done' ? 'progress-done' : ''}`}
                      style={{ width: `${item.progress}%` }}
                    ></div>
                  </div>
                  <span className="upload-item-status">
                    {item.status === 'pending' && 'Waiting'}
                    {item.status === 'uploading' && `${item.progress}%`}
                    {item.status === 'done' && '✅'}
                    {item.status === 'error' && '❌'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="modal-actions">
          {!allDone && (
            <button
              id="upload-start"
              className="btn-primary"
              onClick={startUpload}
              disabled={!hasPending}
            >
              {hasPending ? `Encrypt & Upload (${items.filter(i => i.status === 'pending').length})` : 'Uploading...'}
            </button>
          )}
          {allDone && (
            <button id="upload-done" className="btn-primary" onClick={onClose}>
              Done
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
