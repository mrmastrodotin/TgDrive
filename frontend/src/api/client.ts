import { FileItem, FolderItem, FolderContents, LoginResponse } from '../types';

const API_BASE = '/api';

function getToken(): string | null {
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

// --- Files ---
export async function uploadFile(
  file: File,
  folderId?: string,
  onProgress?: (percent: number) => void
): Promise<FileItem> {
  const token = getToken();
  const formData = new FormData();
  formData.append('file', file);
  if (folderId) formData.append('folder_id', folderId);

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `${API_BASE}/files/upload`);
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100));
      }
    };

    xhr.onload = () => {
      if (xhr.status === 200) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        reject(new Error(xhr.responseText || 'Upload failed'));
      }
    };

    xhr.onerror = () => reject(new Error('Network error'));
    xhr.send(formData);
  });
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
  const token = getToken();
  const res = await fetch(getDownloadURL(fileId), {
    headers: token ? { 'Authorization': `Bearer ${token}` } : {},
  });
  if (!res.ok) throw new Error('Download failed');

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
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
