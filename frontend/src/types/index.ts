export interface FileItem {
  id: string;
  name: string;
  size: number;
  mime_type: string;
  folder_id: string | null;
  encryption_salt: string;
  chunk_count: number;
  created_at: string;
}

export interface FolderItem {
  id: string;
  name: string;
  parent_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface FolderContents {
  folder: FolderItem | null;
  folders: FolderItem[];
  files: FileItem[];
}

export interface LoginResponse {
  token: string;
  expires_at: number;
}

export type PreviewableType = 'image' | 'video' | 'audio' | 'pdf' | 'text' | 'code' | 'none';

export function getPreviewType(mimeType: string, filename: string): PreviewableType {
  if (mimeType.startsWith('image/')) return 'image';
  if (mimeType.startsWith('video/')) return 'video';
  if (mimeType.startsWith('audio/')) return 'audio';
  if (mimeType === 'application/pdf') return 'pdf';
  if (mimeType.startsWith('text/')) return 'text';

  const codeExts = ['.js', '.ts', '.jsx', '.tsx', '.py', '.go', '.rs', '.c', '.cpp', '.h', '.java', '.json', '.yaml', '.yml', '.toml', '.xml', '.html', '.css', '.scss', '.md', '.sql', '.sh', '.bat', '.ps1'];
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  if (codeExts.includes(ext)) return 'code';

  return 'none';
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

export function getFileIcon(mimeType: string): string {
  if (mimeType.startsWith('image/')) return '🖼️';
  if (mimeType.startsWith('video/')) return '🎬';
  if (mimeType.startsWith('audio/')) return '🎵';
  if (mimeType === 'application/pdf') return '📕';
  if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) return '📊';
  if (mimeType.includes('document') || mimeType.includes('word')) return '📝';
  if (mimeType.includes('presentation') || mimeType.includes('powerpoint')) return '📽️';
  if (mimeType.includes('zip') || mimeType.includes('archive') || mimeType.includes('compressed')) return '📦';
  if (mimeType.startsWith('text/')) return '📄';
  return '📄';
}
