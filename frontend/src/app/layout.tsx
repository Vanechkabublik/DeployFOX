import type { Metadata } from "next";
import localFont from "next/font/local";
import "@/styles/globals.css";
const montserrat = localFont({
  src: "./fonts/Montserrat.ttf",
  display: "swap",
  variable: "--font-montserrat",
});
export const metadata: Metadata = {
  title: "DeployFox — Your code. Your server.",
  description: "Self-hosted deployment control panel",
};
export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body className={montserrat.variable}>{children}</body>
    </html>
  );
}
