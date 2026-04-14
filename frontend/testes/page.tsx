// src/app/(painel)/dev/seed/page.tsx
import { Metadata } from "next";
import PageContent from "./PageContent";

export const metadata: Metadata = {
  title: "Painel de Testes",
};

export default function DevSeedPage() {
  return <PageContent />;
}
