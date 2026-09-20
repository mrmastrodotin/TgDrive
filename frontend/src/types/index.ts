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

export type FileCategory = 'all' | 'media' | 'docs' | 'code' | 'archives';

export function getFileCategory(mimeType: string, filename: string): FileCategory {
  if (mimeType.startsWith('image/') || mimeType.startsWith('video/') || mimeType.startsWith('audio/')) {
    return 'media';
  }
  if (mimeType === 'application/pdf' || mimeType.includes('document') || mimeType.includes('word') || mimeType.includes('sheet') || mimeType.includes('presentation')) {
    return 'docs';
  }
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  const codeExts = ['.js', '.ts', '.jsx', '.tsx', '.py', '.go', '.rs', '.c', '.cpp', '.h', '.java', '.json', '.yaml', '.yml', '.toml', '.xml', '.html', '.css', '.scss', '.md', '.sql', '.sh', '.bat', '.ps1'];
  if (codeExts.includes(ext) || mimeType.startsWith('text/')) {
    return 'code';
  }
  const archExts = ['.zip', '.tar', '.gz', '.7z', '.rar', '.bz2', '.iso'];
  if (archExts.includes(ext) || mimeType.includes('zip') || mimeType.includes('archive') || mimeType.includes('compressed')) {
    return 'archives';
  }
  return 'docs';
}

export function getPreviewType(mimeType: string, filename: string): PreviewableType {
  if (mimeType.startsWith('image/')) return 'image';
  if (mimeType.startsWith('video/')) return 'video';
  if (mimeType.startsWith('audio/')) return 'audio';
  if (mimeType === 'application/pdf') return 'pdf';

  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  const codeExts = ['.js', '.ts', '.jsx', '.tsx', '.py', '.go', '.rs', '.c', '.cpp', '.h', '.java', '.json', '.yaml', '.yml', '.toml', '.xml', '.html', '.css', '.scss', '.md', '.sql', '.sh', '.bat', '.ps1'];
  if (codeExts.includes(ext)) return 'code';
  if (mimeType.startsWith('text/')) return 'text';

  return 'none';
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}
