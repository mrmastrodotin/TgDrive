import {
  FileItem,
  FolderItem,
  FolderContents,
  LoginResponse,
  UploadSessionInitResponse,
  UploadStatusResponse,
  BotPoolStatusResponse,
} from '../types';

const API_BASE = '/api';

export function getToken(): string | null {
  return localStorage.getItem('tgdrive_token');
}

function setToken(token: string, expiresAt: number): void {
  localStorage.setItem('tgdrive_token', token);
  localStorage.setItem('tgdrive_token_expires', String(expiresAt));
}

export function clearToken(): void {
  localStorage.removeItem('tgdrive_token');
  localStorage.removeItem('tgdrive_token_expires');
}

export function isAuthenticated(): boolean {
  const token = getToken();
  const expires = localStorage.getItem('tgdrive_token_expires');
  if (!token || !expires) return false;
  return Date.now() / 1000 < Number(expires);
}

async function fetchAPI(path: string, options: RequestInit = {}): Promise<Response> {
  const token = getToken();
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> || {}),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });

  if (res.status === 401) {
    clearToken();
    window.location.reload();
    throw new Error('Session expired');
  }

  return res;
}

// --- Auth ---
export async function login(password: string): Promise<LoginResponse> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
  if (!res.ok) throw new Error('Invalid password');
  const data: LoginResponse = await res.json();
  setToken(data.token, data.expires_at);
  return data;
}

// --- Folders ---
export async function listFolder(parentId?: string): Promise<FolderContents> {
  const params = parentId ? `?parent_id=${parentId}` : '';
  const res = await fetchAPI(`/folders${params}`);
  if (!res.ok) throw new Error('Failed to list folder');
  return res.json();
}

export async function createFolder(name: string, parentId?: string): Promise<FolderItem> {
  const res = await fetchAPI('/folders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, parent_id: parentId || null }),
  });
  if (!res.ok) throw new Error('Failed to create folder');
  return res.json();
}

export async function deleteFolder(id: string): Promise<void> {
  const res = await fetchAPI(`/folders/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete folder');
}

export async function renameFolder(id: string, name: string): Promise<void> {
  const res = await fetchAPI(`/folders/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error('Failed to rename folder');
}

// --- Metadata Extraction ---
async function extractMediaMetadata(file: File): Promise<{duration?: number, width?: number, height?: number, thumbnailBlob?: Blob}> {
  return new Promise((resolve) => {
    const isVideo = file.type.startsWith('video/');
    const isImage = file.type.startsWith('image/');
    const isAudio = file.type.startsWith('audio/');

    if (!isVideo && !isImage && !isAudio) {
      resolve({});
      return;
    }

    const url = URL.createObjectURL(file);

    if (isAudio) {
      const audio = new Audio(url);
      audio.onloadedmetadata = () => {
        resolve({ duration: audio.duration });
        URL.revokeObjectURL(url);
      };
      audio.onerror = () => resolve({});
      return;
    }

    if (isImage) {
      const img = new Image();
      img.onload = () => {
        const canvas = document.createElement('canvas');
        const maxSize = 320;
        let { width, height } = img;
        if (width > height && width > maxSize) {
          height *= maxSize / width;
          width = maxSize;
        } else if (height > maxSize) {
          width *= maxSize / height;
          height = maxSize;
        }
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        if (ctx) ctx.drawImage(img, 0, 0, width, height);
        
        canvas.toBlob((blob) => {
          resolve({ width: img.naturalWidth, height: img.naturalHeight, thumbnailBlob: blob || undefined });
          URL.revokeObjectURL(url);
        }, 'image/jpeg', 0.8);
      };
      img.onerror = () => resolve({});
      img.src = url;
      return;
    }

    if (isVideo) {
      const video = document.createElement('video');
      video.muted = true;
      video.playsInline = true;
      let duration = 0;
      let width = 0;
      let height = 0;

      video.onloadedmetadata = () => {
        duration = video.duration;
        width = video.videoWidth;
        height = video.videoHeight;
        video.currentTime = Math.min(1, duration / 2); // Seek slightly into the video
      };

      video.onseeked = () => {
        const canvas = document.createElement('canvas');
        const maxSize = 320;
        let w = width;
        let h = height;
        if (w > h && w > maxSize) {
          h *= maxSize / w;
          w = maxSize;
        } else if (h > maxSize) {
          w *= maxSize / h;
          h = maxSize;
        }
        canvas.width = w;
        canvas.height = h;
        const ctx = canvas.getContext('2d');
        if (ctx) ctx.drawImage(video, 0, 0, w, h);
        
        canvas.toBlob((blob) => {
          resolve({ duration, width, height, thumbnailBlob: blob || undefined });
          URL.revokeObjectURL(url);
        }, 'image/jpeg', 0.8);
      };
      video.onerror = () => resolve({});
      video.src = url;
      return;
    }
  });
}

// Helper to convert Blob to Base64 string (without data URL prefix)
function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => {
      const dataUrl = reader.result as string;
      const base64 = dataUrl.split(',')[1] || '';
      resolve(base64);
    };
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

// Compute a fast, deterministic source fingerprint using sampling without whole-file buffering
export async function computeFileFingerprint(file: File): Promise<string> {
  const sliceSize = 64 * 1024;
  const parts: Blob[] = [];

  const metaStr = `${file.name}:${file.size}:${file.lastModified}:`;
  parts.push(new Blob([metaStr]));

  // First 64KB
  parts.push(file.slice(0, Math.min(sliceSize, file.size)));

  // Middle 64KB (if file > 128KB)
  if (file.size > sliceSize * 2) {
    const midStart = Math.floor(file.size / 2) - Math.floor(sliceSize / 2);
    parts.push(file.slice(midStart, midStart + sliceSize));
  }

  // Last 64KB (if file > 64KB)
  if (file.size > sliceSize) {
    const endStart = Math.max(0, file.size - sliceSize);
    parts.push(file.slice(endStart, file.size));
  }

  const combinedBlob = new Blob(parts);
  const buffer = await combinedBlob.arrayBuffer();

  if (typeof crypto !== 'undefined' && crypto.subtle) {
    const hashBuffer = await crypto.subtle.digest('SHA-256', buffer);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
  }

  // Fallback simple hash if crypto.subtle is unavailable
  let hash = 0;
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < bytes.length; i++) {
    hash = (hash << 5) - hash + bytes[i];
    hash |= 0;
  }
  return `${metaStr.replace(/[^a-zA-Z0-9]/g, '_')}_${Math.abs(hash).toString(16)}`;
}

// --- Chunked Upload Session Protocol ---

export async function initUploadSession(params: {
  name: string;
  size: number;
  mime_type: string;
  folder_id?: string;
  chunk_size?: number;
  duration?: number;
  width?: number;
  height?: number;
  thumbnail_base64?: string;
  source_fingerprint?: string;
}): Promise<UploadSessionInitResponse> {
  const res = await fetchAPI('/files/upload/init', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  });
  if (!res.ok) {
    const errText = await res.text().catch(() => '');
    throw new Error(`Init upload failed: ${errText || res.statusText}`);
  }
  return res.json();
}

export async function getUploadStatus(uploadId: string): Promise<UploadStatusResponse> {
  const res = await fetchAPI(`/files/upload/${uploadId}/status`);
  if (!res.ok) throw new Error('Failed to get upload status');
  return res.json();
}

export async function computeBlobSHA256(blob: Blob): Promise<string> {
  const buf = await blob.arrayBuffer();
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    const digest = await crypto.subtle.digest('SHA-256', buf);
    const hashArray = Array.from(new Uint8Array(digest));
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
  }
  return '';
}

export async function uploadChunkBlob(
  uploadId: string,
  chunkIndex: number,
  blob: Blob,
  signal?: AbortSignal,
  sha256Hash?: string
): Promise<void> {
  const token = getToken();
  const hash = sha256Hash || (await computeBlobSHA256(blob));
  const headers: Record<string, string> = {
    'Content-Type': 'application/octet-stream',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
  if (hash) {
    headers['X-Chunk-SHA256'] = hash;
  }

  const res = await fetch(`${API_BASE}/files/upload/${uploadId}/chunks/${chunkIndex}`, {
    method: 'PUT',
    headers,
    body: blob,
    signal,
  });
  if (!res.ok) {
    const errText = await res.text().catch(() => '');
    throw new Error(`Chunk ${chunkIndex} upload failed: ${errText || res.statusText}`);
  }
}

export async function finalizeUploadSession(uploadId: string): Promise<FileItem> {
  const res = await fetchAPI(`/files/upload/${uploadId}/finalize`, {
    method: 'POST',
  });
  if (!res.ok) {
    const errText = await res.text().catch(() => '');
    throw new Error(`Finalize failed: ${errText || res.statusText}`);
  }
  return res.json();
}

export async function cancelUploadSession(uploadId: string): Promise<void> {
  await fetchAPI(`/files/upload/${uploadId}/cancel`, {
    method: 'POST',
  }).catch(() => {});
}

// --- Files Upload (Chunked, Resumable, Source-Aware, Dynamic Concurrency) ---
export async function uploadFile(
  file: File,
  folderId?: string,
  onProgress?: (percent: number) => void,
  signal?: AbortSignal
): Promise<FileItem> {
  let duration: number | undefined;
  let width: number | undefined;
  let height: number | undefined;
  let thumbBase64: string | undefined;

  try {
    const meta = await extractMediaMetadata(file);
    duration = meta.duration;
    width = meta.width;
    height = meta.height;
    if (meta.thumbnailBlob) {
      thumbBase64 = await blobToBase64(meta.thumbnailBlob);
    }
  } catch (err) {
    console.warn('Failed to extract metadata:', err);
  }

  // Calculate validated source identity without reading whole file into memory
  const fingerprint = await computeFileFingerprint(file);
  const resumeKey = `tgdrive_upload_sess_${fingerprint}`;
  let session: UploadSessionInitResponse | null = null;
  let confirmedSet = new Set<number>();

  // Attempt to recover an existing active session with matching fingerprint
  const savedSessionId = localStorage.getItem(resumeKey);
  if (savedSessionId) {
    try {
      const status = await getUploadStatus(savedSessionId);
      // Validate that the session is still active, size matches, and source fingerprint matches
      const fpMatch = !status.source_fingerprint || status.source_fingerprint === fingerprint;
      if (status.status === 'uploading' && status.size === file.size && fpMatch) {
        session = {
          upload_id: status.upload_id,
          file_id: status.file_id,
          chunk_size: status.chunk_size,
          total_chunks: status.total_chunks,
          status: status.status,
        };

        // Bounded per-chunk SHA-256 validation: hash only candidate chunk slices to verify matching content
        const candidateConfirmed = status.confirmed_chunks || [];
        const hashes = status.chunk_hashes || {};

        for (const idx of candidateConfirmed) {
          const expectedHash = hashes[idx];
          if (expectedHash) {
            const start = idx * status.chunk_size;
            const end = Math.min(file.size, start + status.chunk_size);
            const slice = file.slice(start, end);
            const actualHash = await computeBlobSHA256(slice);
            if (actualHash === expectedHash) {
              confirmedSet.add(idx);
            } else {
              console.warn(`Resumed chunk ${idx} digest mismatch (expected ${expectedHash}, got ${actualHash}); re-uploading chunk`);
            }
          } else {
            confirmedSet.add(idx);
          }
        }
      } else {
        localStorage.removeItem(resumeKey);
      }
    } catch {
      localStorage.removeItem(resumeKey);
    }
  }

  // Create new session if no recoverable session exists
  if (!session) {
    session = await initUploadSession({
      name: file.name,
      size: file.size,
      mime_type: file.type || 'application/octet-stream',
      folder_id: folderId,
      duration,
      width,
      height,
      thumbnail_base64: thumbBase64,
      source_fingerprint: fingerprint,
    });
    localStorage.setItem(resumeKey, session.upload_id);
  }

  const totalChunks = session.total_chunks;
  const chunkSize = session.chunk_size;

  const missingIndices: number[] = [];
  for (let i = 0; i < totalChunks; i++) {
    if (!confirmedSet.has(i)) {
      missingIndices.push(i);
    }
  }

  let confirmedCount = confirmedSet.size;
  if (onProgress) {
    onProgress(totalChunks > 0 ? Math.round((confirmedCount / totalChunks) * 100) : 100);
  }

  // Dynamic concurrency pool adapting to backend capacity (clamped between 1 and 6)
  const serverCapacity = session.safe_concurrency || 3;
  const maxSafeConcurrency = 6;
  const concurrency = Math.min(maxSafeConcurrency, Math.max(1, Math.min(serverCapacity, missingIndices.length)));
  let nextMissingIdx = 0;
  let activeWorkers = 0;
  let hasError: Error | null = null;

  if (missingIndices.length > 0) {
    await new Promise<void>((resolve, reject) => {
      function launchNext() {
        if (hasError) return;
        if (signal?.aborted) {
          hasError = new DOMException('Upload cancelled', 'AbortError');
          reject(hasError);
          return;
        }

        if (nextMissingIdx >= missingIndices.length) {
          if (activeWorkers === 0) {
            resolve();
          }
          return;
        }

        const chunkIndex = missingIndices[nextMissingIdx++];
        activeWorkers++;

        const start = chunkIndex * chunkSize;
        const end = Math.min(file.size, start + chunkSize);
        // Bounded slice: only creates a Blob view without copying data into RAM
        const chunkBlob = file.slice(start, end);

        (async () => {
          let lastErr: any;
          for (let attempt = 0; attempt < 3; attempt++) {
            if (signal?.aborted) throw new DOMException('Upload cancelled', 'AbortError');
            try {
              await uploadChunkBlob(session!.upload_id, chunkIndex, chunkBlob, signal);
              lastErr = null;
              break;
            } catch (err: any) {
              lastErr = err;
              if (signal?.aborted) throw err;
              await new Promise((r) => setTimeout(r, (attempt + 1) * 1000));
            }
          }

          if (lastErr) {
            throw lastErr;
          }

          confirmedCount++;
          if (onProgress) {
            onProgress(Math.round((confirmedCount / totalChunks) * 100));
          }
        })()
          .then(() => {
            activeWorkers--;
            launchNext();
          })
          .catch((err) => {
            activeWorkers--;
            if (!hasError) {
              hasError = err;
              reject(err);
            }
          });
      }

      for (let i = 0; i < concurrency; i++) {
        launchNext();
      }
    });
  }

  if (signal?.aborted) {
    cancelUploadSession(session.upload_id).catch(() => {});
    localStorage.removeItem(resumeKey);
    throw new DOMException('Upload cancelled', 'AbortError');
  }

  // Finalize upload session durably
  const finalized = await finalizeUploadSession(session.upload_id);
  localStorage.removeItem(resumeKey);
  if (onProgress) {
    onProgress(100);
  }

  return finalized;
}

export interface StreamTicketResponse {
  ticket: string;
  expires_at: number;
  file_id: string;
}

export async function getScopedStreamTicket(fileId: string): Promise<string> {
  const res = await fetchAPI(`/files/${fileId}/ticket`, {
    method: 'POST',
  });
  if (!res.ok) {
    throw new Error('Failed to acquire playback ticket');
  }
  const data: StreamTicketResponse = await res.json();
  return data.ticket;
}

export async function getScopedStreamURL(fileId: string): Promise<string> {
  try {
    const ticket = await getScopedStreamTicket(fileId);
    return `${API_BASE}/files/${fileId}/stream?ticket=${encodeURIComponent(ticket)}`;
  } catch (err) {
    console.warn('Falling back to standard stream URL:', err);
    return getStreamURL(fileId);
  }
}

export function getDownloadURL(fileId: string): string {
  const token = getToken();
  return token ? `${API_BASE}/files/${fileId}/download?token=${encodeURIComponent(token)}` : `${API_BASE}/files/${fileId}/download`;
}

export function getStreamURL(fileId: string): string {
  const token = getToken();
  return token ? `${API_BASE}/files/${fileId}/stream?token=${encodeURIComponent(token)}` : `${API_BASE}/files/${fileId}/stream`;
}

export function getPreviewURL(fileId: string): string {
  const token = getToken();
  return token ? `${API_BASE}/files/${fileId}/preview?token=${encodeURIComponent(token)}` : `${API_BASE}/files/${fileId}/preview`;
}

export async function downloadFile(fileId: string, filename: string): Promise<void> {
  const url = getDownloadURL(fileId);
  window.location.assign(url);
}

export async function deleteFile(id: string): Promise<void> {
  const res = await fetchAPI(`/files/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete file');
}

export async function getFileInfo(id: string): Promise<FileItem> {
  const res = await fetchAPI(`/files/${id}/info`);
  if (!res.ok) throw new Error('File not found');
  return res.json();
}

export interface WarmupStatus {
  file_id: string;
  total_chunks: number;
  cached_chunks: number[];
  cached_count: number;
  ready: boolean;
}

export async function warmupFile(id: string): Promise<WarmupStatus> {
  const res = await fetchAPI(`/files/${id}/warmup`, { method: 'POST' });
  if (!res.ok) throw new Error('Warmup failed');
  return res.json();
}

export async function getWarmupStatus(id: string): Promise<WarmupStatus> {
  const res = await fetchAPI(`/files/${id}/warmup`);
  if (!res.ok) throw new Error('Warmup status failed');
  return res.json();
}

export async function saveFileProgress(id: string, progress: number): Promise<void> {
  const res = await fetchAPI(`/files/${id}/progress`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ progress }),
  });
  if (!res.ok) throw new Error('Failed to save progress');
}

export async function setFileCinema(id: string, isCinema: boolean): Promise<{ status: string; is_cinema: boolean }> {
  const res = await fetchAPI(`/files/${id}/cinema`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ is_cinema: isCinema }),
  });
  if (!res.ok) throw new Error('Failed to update cinema status');
  return res.json();
}

export interface BotPoolStatus {
  total: number;
  online: number;
}

export async function getBotPoolStatus(): Promise<BotPoolStatus> {
  const res = await fetchAPI('/bots/status');
  if (!res.ok) throw new Error('Failed to fetch bot pool status');
  return res.json();
}

export interface StorageStats {
  total_bytes: number;
  total_files: number;
  video_bytes: number;
  audio_bytes: number;
  image_bytes: number;
  document_bytes: number;
  archive_bytes: number;
  other_bytes: number;
}

export async function getStorageStats(): Promise<StorageStats> {
  const res = await fetchAPI('/stats');
  if (!res.ok) throw new Error('Failed to fetch storage stats');
  return res.json();
}

// --- Dynamic Bot Management ---
export async function getBotsList(): Promise<BotPoolStatusResponse> {
  const res = await fetchAPI('/bots');
  if (!res.ok) throw new Error('Failed to fetch bots');
  return res.json();
}

export async function addBotToken(token: string): Promise<void> {
  const res = await fetchAPI('/bots', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  });
  if (!res.ok) {
    const errText = await res.text().catch(() => '');
    throw new Error(`Failed to add bot: ${errText || res.statusText}`);
  }
}

export async function removeBotToken(tokenHash: string): Promise<void> {
  const res = await fetchAPI(`/bots/${tokenHash}`, {
    method: 'DELETE',
  });
  if (!res.ok) {
    const errText = await res.text().catch(() => '');
    throw new Error(`Failed to remove bot: ${errText || res.statusText}`);
  }
}
