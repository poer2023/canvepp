import type { Metadata } from 'next';
import './globals.css';
export const metadata: Metadata = { title: 'Canvepp · 图像创作画布', description: '把灵感连起来。保留每一次图像创作的输入、版本与选择。' };
export default function RootLayout({ children }: {
    children: React.ReactNode;
}) { return <html lang="zh-CN"><body>{children}</body></html>; }
