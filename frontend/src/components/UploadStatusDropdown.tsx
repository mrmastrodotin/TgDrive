import React, { useState, useRef, useEffect } from 'react';
import { UploadTask } from '../hooks/useUploadQueue';
import { formatFileSize } from '../types';
import {
  UploadCloud,
  CheckCircle2,
  AlertCircle,
  X,
  ChevronDown,
  File as FileIcon,
  Trash2,
  Plus,
  Loader2,
  Ban,
} from 'lucide-react';

interface UploadStatusDropdownProps {
  tasks: UploadTask[];
  onCancel: (id: string) => void;
  onCancelAll: () => void;
  onClearFinished: () => void;
  onOpenUploadModal: () => void;
}

export default function UploadStatusDropdown({
  tasks,
  onCancel,
  onCancelAll,
  onClearFinished,
  onOpenUploadModal,
}: UploadStatusDropdownProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close dropdown on outside click
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      document.addEventListener('mousedown', handleOutsideClick);
    }
    return () => {
      document.removeEventListener('mousedown', handleOutsideClick);
    };
  }, [isOpen]);

  if (tasks.length === 0) {
    return null;
  }

  const activeCount = tasks.filter((t) => t.status === 'uploading' || t.status === 'pending').length;
  const completedCount = tasks.filter((t) => t.status === 'done').length;
  const hasFinished = tasks.some((t) => t.status === 'done' || t.status === 'cancelled' || t.status === 'error');

  const formatETA = (seconds?: number): string => {
    if (!seconds || !isFinite(seconds) || seconds <= 0) return '';
    if (seconds < 60) return `${Math.ceil(seconds)}s left`;
    const m = Math.floor(seconds / 60);
    const s = Math.ceil(seconds % 60);
    return `${m}m ${s}s left`;
  };

  const formatSpeed = (bytesPerSec?: number): string => {
    if (!bytesPerSec || bytesPerSec <= 0) return '';
    if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`;
    if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(0)} KB/s`;
    return `${bytesPerSec.toFixed(0)} B/s`;
  };

  return (
    <div className="relative inline-block" ref={containerRef}>
      {/* Top Bar Upload Status Pill */}
      <button
        type="button"
        onClick={() => setIsOpen((prev) => !prev)}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-full border transition-all select-none ${
          activeCount > 0
            ? 'bg-amber-500/10 border-amber-500/40 text-amber-300 hover:bg-amber-500/20 shadow-[0_0_15px_rgba(252,186,105,0.2)]'
            : 'bg-surface-container-low border-outline-variant/40 text-on-surface-variant hover:text-on-surface hover:bg-surface-container'
        }`}
        title="View upload activity"
      >
        {activeCount > 0 ? (
          <span className="relative flex h-3.5 w-3.5 items-center justify-center">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-60"></span>
            <Loader2 size={14} className="animate-spin text-amber-400" />
          </span>
        ) : (
          <CheckCircle2 size={14} className="text-emerald-400" />
        )}

        <span className="font-label-mono-sm text-xs font-semibold tracking-wide">
          {activeCount > 0
            ? `${activeCount} uploading${completedCount > 0 ? `, ${completedCount} completed` : ''}`
            : `${completedCount} completed`}
        </span>

        <ChevronDown
          size={14}
          className={`transition-transform duration-200 text-amber-400/80 ${
            isOpen ? 'rotate-180' : ''
          }`}
        />
      </button>

      {/* Floating Dropdown Window */}
      {isOpen && (
        <div className="absolute right-0 top-full mt-2.5 w-96 max-w-[92vw] z-50 rounded-2xl bg-[#0e131d]/98 backdrop-blur-2xl border border-amber-500/25 p-4 shadow-[0_16px_50px_rgba(0,0,0,0.85)] animate-in fade-in zoom-in-95 duration-150">
          {/* Dropdown Header */}
          <div className="flex items-center justify-between pb-3 mb-3 border-b border-white/10">
            <div className="flex items-center gap-2">
              <UploadCloud size={18} className="text-amber-400" />
              <h4 className="font-semibold text-sm text-zinc-100">Upload Activity</h4>
              <span className="px-2 py-0.5 rounded-full text-[11px] font-mono bg-amber-500/15 text-amber-300 border border-amber-500/30">
                {tasks.length}
              </span>
            </div>

            <div className="flex items-center gap-2 text-xs">
              {activeCount > 0 && (
                <button
                  type="button"
                  onClick={onCancelAll}
                  className="px-2 py-1 rounded-md text-rose-400 hover:text-rose-300 hover:bg-rose-500/15 transition-colors font-medium"
                >
                  Cancel All
                </button>
              )}
              {hasFinished && (
                <button
                  type="button"
                  onClick={onClearFinished}
                  className="p-1 rounded-md text-zinc-400 hover:text-zinc-200 hover:bg-white/5 transition-colors"
                  title="Clear finished uploads"
                >
                  <Trash2 size={14} />
                </button>
              )}
            </div>
          </div>

          {/* Upload Items Scrollable List */}
          <div className="max-h-72 overflow-y-auto space-y-2 pr-1 custom-scrollbar">
            {tasks.map((task) => {
              const isUploading = task.status === 'uploading';
              const isPending = task.status === 'pending';
              const isDone = task.status === 'done';
              const isCancelled = task.status === 'cancelled';
              const isError = task.status === 'error';

              return (
                <div
                  key={task.id}
                  className={`p-2.5 rounded-xl border transition-all ${
                    isUploading
                      ? 'bg-amber-500/[0.06] border-amber-500/30'
                      : isDone
                      ? 'bg-emerald-500/[0.04] border-emerald-500/20'
                      : isCancelled
                      ? 'bg-white/[0.02] border-white/5 opacity-60'
                      : isError
                      ? 'bg-rose-500/[0.06] border-rose-500/25'
                      : 'bg-white/[0.03] border-white/5'
                  }`}
                >
                  {/* Top line: File info & status / cancel button */}
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0">
                      <FileIcon
                        size={16}
                        className={`flex-shrink-0 ${
                          isUploading
                            ? 'text-amber-400'
                            : isDone
                            ? 'text-emerald-400'
                            : isError
                            ? 'text-rose-400'
                            : 'text-zinc-400'
                        }`}
                      />
                      <div className="truncate">
                        <p className="text-xs font-medium text-zinc-100 truncate" title={task.file.name}>
                          {task.file.name}
                        </p>
                        <p className="text-[11px] text-zinc-400 font-mono">
                          {formatFileSize(task.file.size)}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 flex-shrink-0">
                      {isDone && (
                        <span className="flex items-center gap-1 text-[11px] font-medium text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-md">
                          <CheckCircle2 size={12} />
                          <span>Done</span>
                        </span>
                      )}

                      {isPending && (
                        <span className="text-[11px] text-zinc-400 bg-white/5 px-2 py-0.5 rounded-md">
                          Queued
                        </span>
                      )}

                      {isCancelled && (
                        <span className="flex items-center gap-1 text-[11px] text-zinc-400 bg-white/5 px-2 py-0.5 rounded-md">
                          <Ban size={12} />
                          <span>Cancelled</span>
                        </span>
                      )}

                      {isError && (
                        <span
                          className="flex items-center gap-1 text-[11px] text-rose-400 bg-rose-500/10 px-2 py-0.5 rounded-md"
                          title={task.error}
                        >
                          <AlertCircle size={12} />
                          <span>Failed</span>
                        </span>
                      )}

                      {/* Cancel Button for in-flight or queued uploads */}
                      {(isUploading || isPending) && (
                        <button
                          type="button"
                          onClick={() => onCancel(task.id)}
                          className="p-1 rounded-lg text-zinc-400 hover:text-rose-400 hover:bg-rose-500/15 transition-colors"
                          title="Cancel this upload"
                        >
                          <X size={15} />
                        </button>
                      )}
                    </div>
                  </div>

                  {/* Progress Bar & Telemetry for active uploading */}
                  {isUploading && (
                    <div className="mt-2.5 space-y-1.5">
                      <div className="w-full bg-white/10 rounded-full h-1.5 overflow-hidden">
                        <div
                          className="bg-gradient-to-r from-amber-400 to-amber-600 h-full rounded-full transition-all duration-200"
                          style={{ width: `${task.progress}%` }}
                        />
                      </div>

                      <div className="flex items-center justify-between text-[11px] font-mono text-zinc-400">
                        <span className="text-amber-300 font-semibold">{task.progress}%</span>
                        <div className="flex items-center gap-2 text-[10.5px]">
                          {task.speed && task.speed > 0 && <span>{formatSpeed(task.speed)}</span>}
                          {task.eta && task.eta > 0 && <span>· {formatETA(task.eta)}</span>}
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {/* Footer: Add More Files */}
          <div className="mt-3 pt-3 border-t border-white/10 flex items-center justify-between">
            <button
              type="button"
              onClick={() => {
                setIsOpen(false);
                onOpenUploadModal();
              }}
              className="flex items-center gap-1.5 text-xs text-amber-300 hover:text-amber-200 hover:underline font-medium"
            >
              <Plus size={14} />
              <span>Upload more files</span>
            </button>
            <span className="text-[11px] text-zinc-400 font-mono">
              AES-256 Encrypted
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
