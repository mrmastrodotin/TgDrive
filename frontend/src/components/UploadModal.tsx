import React, { useState, useRef, useCallback } from 'react';
import { formatFileSize } from '../types';
import {
  UploadCloud,
  X,
  File,
  Lock,
  Plus,
  Trash2,
  ShieldCheck,
} from 'lucide-react';

interface UploadModalProps {
  folderId?: string;
  onClose: () => void;
  onStartUpload?: (files: File[]) => void;
  onUploadComplete?: () => void;
}

interface StagedFile {
  file: File;
  id: string;
}

export default function UploadModal({ onClose, onStartUpload, onUploadComplete }: UploadModalProps) {
  const [stagedFiles, setStagedFiles] = useState<StagedFile[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const addFiles = (files: FileList | File[]) => {
    const newItems: StagedFile[] = Array.from(files).map((file) => ({
      file,
      id: `${file.name}-${file.size}-${Date.now()}-${Math.random()}`,
    }));
    setStagedFiles((prev) => [...prev, ...newItems]);
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

  const removeFile = (id: string) => {
    setStagedFiles((prev) => prev.filter((item) => item.id !== id));
  };

  const clearAll = () => {
    setStagedFiles([]);
  };

  // Automatically start upload and hide the modal window
  const handleUploadSubmit = () => {
    if (stagedFiles.length === 0) return;
    const filesToUpload = stagedFiles.map((item) => item.file);
    if (onStartUpload) {
      onStartUpload(filesToUpload);
    }
    if (onUploadComplete) {
      onUploadComplete();
    }
    onClose(); // Auto-hides popup immediately!
  };

  const totalSize = stagedFiles.reduce((acc, curr) => acc + curr.file.size, 0);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="w-full max-w-xl rounded-3xl bg-[#0f131c]/98 backdrop-blur-2xl border border-amber-500/25 p-6 md:p-7 flex flex-col gap-5 shadow-[0_25px_60px_rgba(0,0,0,0.85)] shadow-amber-500/5 animate-in zoom-in-95 duration-200"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal Header */}
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-amber-500/15 border border-amber-500/30 text-amber-400 flex items-center justify-center shadow-[0_0_15px_rgba(252,186,105,0.2)]">
              <UploadCloud size={22} />
            </div>
            <div>
              <h3 className="text-lg font-bold text-zinc-100 tracking-tight">Upload & Encrypt</h3>
              <p className="text-xs text-zinc-400 mt-0.5">
                Files are 20MB chunked & AES-256 encrypted before transmission
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-xl text-zinc-400 hover:text-zinc-100 hover:bg-white/10 transition-colors"
          >
            <X size={18} />
          </button>
        </div>

        {/* Dropzone */}
        <div
          className={`border-2 border-dashed rounded-2xl p-6 text-center cursor-pointer transition-all ${
            isDragging
              ? 'border-amber-400 bg-amber-500/10 shadow-[inset_0_0_25px_rgba(252,186,105,0.15)] scale-[1.01]'
              : 'border-amber-500/30 bg-amber-500/[0.02] hover:border-amber-400/70 hover:bg-amber-500/[0.04]'
          }`}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
        >
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => e.target.files && addFiles(e.target.files)}
          />
          <div className="w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center mx-auto mb-3 shadow-[0_0_15px_rgba(252,186,105,0.15)]">
            <UploadCloud size={24} />
          </div>
          <h4 className="text-sm font-semibold text-zinc-100 mb-1">
            Drag & drop files here, or{' '}
            <span className="text-amber-400 underline underline-offset-4 hover:text-amber-300">
              Browse
            </span>
          </h4>
          <p className="text-xs text-zinc-400">
            Supports 4K video, audio, archives, documents & code up to 2GB per file
          </p>
        </div>

        {/* Selected Files Queue */}
        {stagedFiles.length > 0 && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs font-medium text-zinc-300 px-1">
              <span>
                Selected Files ({stagedFiles.length}) ·{' '}
                <span className="text-amber-300 font-mono">{formatFileSize(totalSize)}</span>
              </span>
              <button
                type="button"
                onClick={clearAll}
                className="flex items-center gap-1 text-zinc-400 hover:text-rose-400 transition-colors"
              >
                <Trash2 size={12} />
                <span>Clear</span>
              </button>
            </div>

            <div className="max-h-48 overflow-y-auto space-y-2 pr-1 custom-scrollbar">
              {stagedFiles.map((item) => {
                const chunksEst = Math.max(1, Math.ceil(item.file.size / (16 * 1024 * 1024)));
                return (
                  <div
                    key={item.id}
                    className="p-2.5 rounded-xl bg-white/[0.03] border border-white/5 flex items-center justify-between gap-3 hover:border-amber-500/20 transition-all"
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <File size={16} className="text-amber-400 flex-shrink-0" />
                      <div className="truncate">
                        <p className="text-xs font-medium text-zinc-100 truncate" title={item.file.name}>
                          {item.file.name}
                        </p>
                        <p className="text-[11px] text-zinc-400 font-mono">
                          {formatFileSize(item.file.size)}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 flex-shrink-0">
                      <span className="flex items-center gap-1 text-[10px] font-mono text-amber-300/80 bg-amber-500/10 border border-amber-500/20 px-2 py-0.5 rounded-md">
                        <Lock size={10} />
                        <span>
                          {chunksEst} {chunksEst === 1 ? 'chunk' : 'chunks'}
                        </span>
                      </span>

                      <button
                        type="button"
                        onClick={() => removeFile(item.id)}
                        className="p-1 rounded-lg text-zinc-400 hover:text-rose-400 hover:bg-rose-500/15 transition-colors"
                        title="Remove file"
                      >
                        <X size={14} />
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Modal Footer */}
        <div className="flex items-center justify-between pt-3 border-t border-white/10">
          <div className="flex items-center gap-1.5 text-xs text-zinc-400">
            <ShieldCheck size={14} className="text-emerald-400" />
            <span>Encrypted in memory · Zero leaks</span>
          </div>

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-semibold text-zinc-300 hover:text-white hover:bg-white/5 border border-white/10 transition-colors"
            >
              Cancel
            </button>

            <button
              type="button"
              onClick={handleUploadSubmit}
              disabled={stagedFiles.length === 0}
              className={`flex items-center gap-1.5 px-5 py-2 rounded-xl text-xs font-bold transition-all shadow-[0_0_15px_rgba(252,186,105,0.25)] ${
                stagedFiles.length > 0
                  ? 'bg-gradient-to-r from-amber-400 to-amber-600 hover:from-amber-300 hover:to-amber-500 text-black cursor-pointer'
                  : 'bg-zinc-800 text-zinc-500 cursor-not-allowed border border-white/5'
              }`}
            >
              <Plus size={15} />
              <span>
                Upload {stagedFiles.length > 0 ? `(${stagedFiles.length})` : '& Encrypt'}
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
