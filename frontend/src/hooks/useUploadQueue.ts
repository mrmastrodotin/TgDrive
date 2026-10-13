import { useState, useRef, useEffect, useCallback } from 'react';
import { uploadFile } from '../api/client';
import { FileItem } from '../types';

export interface UploadTask {
  id: string;
  file: File;
  folderId?: string;
  progress: number;
  status: 'pending' | 'uploading' | 'done' | 'error' | 'cancelled';
  error?: string;
  startedAt?: number;
  bytesLoaded?: number;
  speed?: number;     // bytes per second
  eta?: number;       // seconds remaining
  abortController?: AbortController;
}

interface UseUploadQueueOptions {
  onFileUploaded?: (fileItem: FileItem, task: UploadTask) => void;
  onError?: (error: Error, task: UploadTask) => void;
  onCancel?: (task: UploadTask) => void;
}

export function useUploadQueue(options?: UseUploadQueueOptions) {
  const [tasks, setTasks] = useState<UploadTask[]>([]);
  const tasksRef = useRef<UploadTask[]>([]);
  tasksRef.current = tasks;

  const isProcessingRef = useRef(false);
  const optionsRef = useRef(options);
  optionsRef.current = options;

  // Update a single task in state & ref
  const updateTask = useCallback((id: string, patch: Partial<UploadTask>) => {
    setTasks((prev) =>
      prev.map((t) => (t.id === id ? { ...t, ...patch } : t))
    );
  }, []);

  // Process the queue
  const processQueue = useCallback(async () => {
    if (isProcessingRef.current) return;
    isProcessingRef.current = true;

    try {
      while (true) {
        // Find next pending task
        const currentTasks = tasksRef.current;
        const nextTask = currentTasks.find((t) => t.status === 'pending');
        if (!nextTask) break;

        const controller = new AbortController();
        const startedAt = Date.now();

        // Mark as uploading
        updateTask(nextTask.id, {
          status: 'uploading',
          startedAt,
          bytesLoaded: 0,
          progress: 0,
          abortController: controller,
        });

        // Wait a tick for state to settle
        await new Promise((r) => setTimeout(r, 20));

        try {
          const fileSize = nextTask.file.size;
          const uploadedItem = await uploadFile(
            nextTask.file,
            nextTask.folderId,
            (percent) => {
              const loaded = Math.round((percent / 100) * fileSize);
              const elapsed = Math.max(0.2, (Date.now() - startedAt) / 1000);
              const speed = loaded / elapsed;
              const remaining = Math.max(0, fileSize - loaded);
              const eta = speed > 0 ? remaining / speed : 0;

              updateTask(nextTask.id, {
                progress: percent,
                bytesLoaded: loaded,
                speed,
                eta,
              });
            },
            controller.signal
          );

          updateTask(nextTask.id, {
            status: 'done',
            progress: 100,
            bytesLoaded: fileSize,
            speed: 0,
            eta: 0,
          });

          if (optionsRef.current?.onFileUploaded) {
            optionsRef.current.onFileUploaded(uploadedItem, nextTask);
          }
        } catch (err: any) {
          const isAborted =
            controller.signal.aborted ||
            err.name === 'AbortError' ||
            (err.message && err.message.toLowerCase().includes('cancel'));

          if (isAborted) {
            updateTask(nextTask.id, {
              status: 'cancelled',
              error: 'Upload cancelled by user',
              speed: 0,
              eta: 0,
            });
            if (optionsRef.current?.onCancel) {
              optionsRef.current.onCancel(nextTask);
            }
          } else {
            updateTask(nextTask.id, {
              status: 'error',
              error: err.message || 'Upload failed',
              speed: 0,
              eta: 0,
            });
            if (optionsRef.current?.onError) {
              optionsRef.current.onError(err, nextTask);
            }
          }
        }
      }
    } finally {
      isProcessingRef.current = false;
    }
  }, [updateTask]);

  // Trigger processing whenever tasks change and there are pending items
  useEffect(() => {
    const hasPending = tasks.some((t) => t.status === 'pending');
    if (hasPending && !isProcessingRef.current) {
      processQueue();
    }
  }, [tasks, processQueue]);

  // Public: Start uploading a batch of files
  const startUploads = useCallback((files: File[], folderId?: string) => {
    if (!files || files.length === 0) return;

    const newTasks: UploadTask[] = files.map((file, idx) => ({
      id: `up-${Date.now()}-${idx}-${Math.random().toString(36).substring(2, 7)}`,
      file,
      folderId,
      progress: 0,
      status: 'pending',
    }));

    setTasks((prev) => [...prev, ...newTasks]);
  }, []);

  // Public: Cancel a specific upload
  const cancelUpload = useCallback((taskId: string) => {
    const target = tasksRef.current.find((t) => t.id === taskId);
    if (!target) return;

    if (target.status === 'uploading') {
      target.abortController?.abort();
    } else if (target.status === 'pending') {
      updateTask(taskId, {
        status: 'cancelled',
        error: 'Upload cancelled',
      });
      if (optionsRef.current?.onCancel) {
        optionsRef.current.onCancel(target);
      }
    }
  }, [updateTask]);

  // Public: Cancel all active/pending uploads
  const cancelAll = useCallback(() => {
    tasksRef.current.forEach((t) => {
      if (t.status === 'uploading') {
        t.abortController?.abort();
      } else if (t.status === 'pending') {
        updateTask(t.id, {
          status: 'cancelled',
          error: 'Upload cancelled',
        });
      }
    });
  }, [updateTask]);

  // Public: Clear completed, cancelled, or errored tasks
  const clearFinished = useCallback(() => {
    setTasks((prev) => prev.filter((t) => t.status === 'pending' || t.status === 'uploading'));
  }, []);

  // Aggregate metrics
  const activeCount = tasks.filter((t) => t.status === 'uploading' || t.status === 'pending').length;
  const completedCount = tasks.filter((t) => t.status === 'done').length;
  const errorCount = tasks.filter((t) => t.status === 'error').length;
  const cancelledCount = tasks.filter((t) => t.status === 'cancelled').length;

  return {
    tasks,
    startUploads,
    cancelUpload,
    cancelAll,
    clearFinished,
    activeCount,
    completedCount,
    errorCount,
    cancelledCount,
    hasActive: activeCount > 0,
  };
}
