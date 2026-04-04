// src/app/(painel)/dev/seed/page.tsx
import { Metadata } from "next";
import SeedTestPage from "./SeedTestPage";

export const metadata: Metadata = {
  title: "Painel de Testes",
};

export default function DevSeedPage() {
  return <SeedTestPage />;
}
