import type { Metadata } from "next";
import "./style.css";
export const metadata: Metadata = { title: "Smart Door", description: "Smart door status dashboard" };
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
