import type { Metadata } from 'next';
import '@/styles/admin.css';

export const metadata: Metadata = {
  title: 'Admin | Enerzia Future Farm',
  description: 'Enerzia Future Farm catalogue console.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
