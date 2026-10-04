import React, { useState, useRef, useCallback, useMemo } from 'react';
import { uploadFile } from '../api/client';
import { formatFileSize } from '../types';
import {
  UploadCloud,
  X,
  File,
  CheckCircle2,
  AlertCircle,
  Lock,
  Plus,
  ArrowUpRight,
} from 'lucide-react';

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
  startedAt?: number;      // timestamp when upload began
  bytesLoaded?: number;    // bytes sent so far
}

export default function UploadModal({ folderId, onClose, onUploadComplete }: UploadModalProps) {
  const [items, setItems] = useState<UploadItem[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
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

  const removeFile = (index: number) => {
    setItems((prev) => prev.filter((_, i) => i !== index));
  };

  const formatETA = (seconds: number): string => {
    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 60) return `${Math.ceil(seconds)}s left`;
    const m = Math.floor(seconds / 60);
    const s = Math.ceil(seconds % 60);
    if (m < 60) return `${m}m ${s}s left`;
    const h = Math.floor(m / 60);
    return `${h}h ${m % 60}m left`;
  };

  const formatSpeed = (bytesPerSec: number): string => {
    if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`;
    if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(0)} KB/s`;
    return `${bytesPerSec.toFixed(0)} B/s`;
  };

  const startUpload = async () => {
    setIsUploading(true);
    for (let i = 0; i < items.length; i++) {
      if (items[i].status !== 'pending') continue;

      const startedAt = Date.now();
      setItems((prev) => {
        const next = [...prev];
        next[i] = { ...next[i], status: 'uploading', startedAt, bytesLoaded: 0 };
        return next;
      });

      try {
        const fileSize = items[i].file.size;
        await uploadFile(items[i].file, folderId, (percent) => {
          const loaded = Math.round((percent / 100) * fileSize);
          setItems((prev) => {
            const next = [...prev];
            next[i] = { ...next[i], progress: percent, bytesLoaded: loaded };
            return next;
          });
        });

        setItems((prev) => {
          const next = [...prev];
          next[i] = { ...next[i], status: 'done', progress: 100, bytesLoaded: fileSize };
          return next;
        });
      } catch (err: any) {
        setItems((prev) => {
          const next = [...prev];
          next[i] = {
            ...next[i],
            status: 'error',
            error: err.message || 'Upload failed',
          };
          return next;
        });
      }
    }
    setIsUploading(false);
    onUploadComplete();
  };

  const pendingCount = items.filter((i) => i.status === 'pending').length;
  const completedCount = items.filter((i) => i.status === 'done').length;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="upload-modal-glass" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header-bar">
          <div className="modal-header-info">
            <h3 className="modal-headline">Upload & Encrypt</h3>
            <p className="modal-subline">Files are split into 20MB chunks and AES-256 encrypted before transmission</p>
          </div>
          <button className="modal-close-btn" onClick={onClose} disabled={isUploading}>
            <X size={20} />
          </button>
        </div>

        {/* Drop Zone */}
        <div
          className={`dropzone-glass ${isDragging ? 'dropzone-active' : ''}`}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
        >
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden-file-input"
            onChange={(e) => e.target.files && addFiles(e.target.files)}
          />
          <div className="dropzone-icon-glow">
            <UploadCloud size={40} className="upload-cloud-icon" />
          </div>
          <h4 className="dropzone-prompt">Drag & Drop files here, or <span className="text-highlight">Browse</span></h4>
          <p className="dropzone-hint">Supports all file types (4K video, documents, archives, audio, code)</p>
        </div>

        {/* Upload File Queue */}
        {items.length > 0 && (
          <div className="upload-queue-container">
            <div className="queue-header">
              <span className="queue-title">Selected Files ({items.length})</span>
              <span className="queue-status-text">
                {completedCount} of {items.length} uploaded
              </span>
            </div>

            <div className="queue-list-scroll">
              {items.map((item, index) => {
                const chunksEst = Math.max(1, Math.ceil(item.file.size / (20 * 1024 * 1024)));
                return (
                  <div key={index} className={`queue-card ${item.status}`}>
                    <div className="queue-card-top">
                      <div className="queue-file-meta">
                        <File size={18} className="queue-file-icon" />
                        <span className="queue-filename" title={item.file.name}>{item.file.name}</span>
                        <span className="queue-filesize">{formatFileSize(item.file.size)}</span>
                      </div>

                      <div className="queue-card-right">
                        <span className="queue-chunk-pill">
                          <Lock size={10} />
                          {chunksEst} {chunksEst === 1 ? 'chunk' : 'chunks'}
                        </span>
                        {item.status === 'pending' && !isUploading && (
                          <button className="queue-remove-btn" onClick={(e) => { e.stopPropagation(); removeFile(index); }}>
                            <X size={14} />
                          </button>
                        )}
                        {item.status === 'done' && (
                          <CheckCircle2 size={18} className="text-emerald" />
                        )}
                        {item.status === 'error' && (
                          <span title={item.error}>
                            <AlertCircle size={18} className="text-rose" />
                          </span>
                        )}
                      </div>
                    </div>

                    {item.status === 'uploading' && (() => {
                      const elapsed = (Date.now() - (item.startedAt || Date.now())) / 1000;
                      const speed = elapsed > 0.5 ? (item.bytesLoaded || 0) / elapsed : 0;
                      const remaining = item.file.size - (item.bytesLoaded || 0);
                      const eta = speed > 0 ? remaining / speed : 0;
                      return (
                        <div className="queue-progress-bar-wrap">
                          <div className="queue-progress-track">
                            <div
                              className="queue-progress-fill"
                              style={{ width: `${item.progress}%` }}
                            ></div>
                          </div>
                          <div className="queue-progress-info">
                            <span className="queue-progress-pct">{item.progress}%</span>
                            {speed > 0 && (
                              <span className="queue-progress-eta">
                                {formatSpeed(speed)} · {formatETA(eta)}
                              </span>
                            )}
                          </div>
                        </div>
                      );
                    })()}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Footer Actions */}
        <div className="modal-footer-row">
          <div className="footer-crypto-badge">
            <Lock size={12} className="text-emerald" />
            <span>Encrypted locally in RAM</span>
          </div>

          <div className="footer-button-group">
            <button className="btn-secondary-glass" onClick={onClose} disabled={isUploading}>
              Cancel
            </button>
            <button
              className="btn-primary-gradient"
              onClick={startUpload}
              disabled={pendingCount === 0 || isUploading}
            >
              {isUploading ? (
                <span className="flex-center-gap">
                  <span className="spinner-border"></span>
                  <span>Encrypting & Sending...</span>
                </span>
              ) : (
                <span className="flex-center-gap">
                  <Plus size={16} />
                  <span>Upload {pendingCount > 0 ? `(${pendingCount})` : ''}</span>
                </span>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
