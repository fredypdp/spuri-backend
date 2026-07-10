---
criado: 2026-07-10 00:00
origem: solicitação do usuário
status: pendente
---

# Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos

## Prompt recomendado para executar a atualização

Implemente a substituição do Google Drive pelo Mega no gerenciamento de arquivos do backend, escolhendo uma biblioteca que permita autenticação por e-mail e senha da conta Mega e suporte, no mínimo, leitura/listagem, criação e organização de pastas, upload, deleção, movimentação/renomeação e, quando disponível, download de arquivos. A mudança não deve alterar a grande estrutura do sistema nem os contratos de negócio já existentes: concentre a alteração na camada responsável por armazenamento/gerenciamento de arquivos e adote uma arquitetura de provider/adapter que permita trocar o provedor de armazenamento de forma rápida, simples, segura e eficiente no futuro.

## Contexto

O sistema atualmente possui integrações e documentação associadas ao Google Drive para armazenar documentos e arquivos gerados/enviados pelo backend. A nova regra de infraestrutura é substituir o Google Drive pelo Mega como provedor principal de armazenamento, sem espalhar dependências específicas do Mega pelo domínio, handlers, casos de uso ou projeções.

A atualização deve preservar a lógica de negócio existente sobre diretórios, arquivos, documentos obrigatórios, caminhos remotos e auditoria, mudando apenas a implementação concreta de storage. Também deve aproveitar a mudança para reduzir acoplamento com o provedor atual, criando uma fronteira clara entre o backend e qualquer serviço externo de arquivos.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Provedor de arquivos | Substituir Google Drive por Mega | Todos os uploads, leituras, movimentações e deleções passam a usar Mega |
| Autenticação | E-mail e senha da conta Mega | Configuração simples via variáveis de ambiente/segredos |
| Biblioteca | Escolher lib com operações essenciais | Suportar leitura/listagem, upload, deleção, pastas, movimentação e idealmente download |
| Arquitetura | Criar interface/adapter de storage | Domínio e handlers não dependem diretamente de Mega ou Google Drive |
| Estrutura do sistema | Preservar contratos e fluxos principais | Não reescrever módulos de negócio desnecessariamente |
| Migração futura | Facilitar troca de provedor | Provider configurável e implementação isolada |
| Legado | Remover dependência operacional do Google Drive | Não manter chamadas ativas ao Drive no fluxo principal |

---

# 1. Escolher biblioteca Mega adequada

## Objetivo

Selecionar e integrar uma biblioteca Mega compatível com o backend que permita gerenciar arquivos e diretórios usando autenticação por e-mail e senha.

## Regra técnica

A biblioteca escolhida deve permitir, obrigatoriamente ou com composição simples:

1. autenticação usando e-mail e senha da conta Mega;
2. criação de pastas;
3. localização/listagem de arquivos e pastas;
4. upload de arquivos;
5. deleção de arquivos e pastas;
6. movimentação de arquivos entre diretórios;
7. renomeação ou atualização de localização quando necessário;
8. download ou geração de fluxo de leitura quando suportado pela lib, ainda que download não seja a prioridade inicial;
9. tratamento explícito de erros de autenticação, permissão, inexistência de arquivo/pasta, limite de quota, rate limit e falha de rede.

## Escopo obrigatório

### 1.1 Avaliar bibliotecas compatíveis

Pesquisar e comparar bibliotecas Mega disponíveis para a linguagem/runtime do backend. A escolha deve considerar:

- manutenção recente;
- suporte às operações exigidas;
- suporte a autenticação com e-mail e senha;
- estabilidade da API;
- compatibilidade de licença;
- capacidade de operar com streams ou arquivos temporários;
- clareza para tratamento de erros;
- facilidade de testes com mocks/fakes.

Registrar a decisão em documentação técnica do projeto, incluindo o motivo da escolha e limitações conhecidas.

### 1.2 Definir configuração por ambiente

Adicionar configuração segura para Mega, preferencialmente por variáveis de ambiente/segredos:

```text
MEGA_EMAIL=
MEGA_PASSWORD=
MEGA_ROOT_FOLDER=
STORAGE_PROVIDER=mega
```

Os nomes finais devem seguir o padrão de configuração existente no projeto. Não hardcodar credenciais, caminhos sensíveis ou identificadores de conta.

### 1.3 Validar inicialização

Na inicialização do cliente Mega:

1. validar presença das credenciais quando `STORAGE_PROVIDER=mega`;
2. autenticar uma única vez ou usar mecanismo eficiente suportado pela lib;
3. falhar de forma explícita e observável quando a autenticação estiver inválida;
4. não expor senha, token ou detalhes sensíveis em logs/respostas;
5. permitir testes sem conexão real usando interface mockada.

---

# 2. Criar abstração de storage/provider

## Objetivo

Isolar o sistema de detalhes específicos do Mega e remover acoplamento direto com Google Drive, adotando um modelo que facilite troca de provedor no futuro.

## Regra de arquitetura

Handlers, domínio, aggregates, projeções e casos de uso não devem importar diretamente bibliotecas do Mega nem do Google Drive. Essas dependências devem ficar restritas à camada de infraestrutura/adapters.

A aplicação deve depender de uma interface de armazenamento com métodos orientados ao negócio, por exemplo:

```go
type FileStorage interface {
    EnsureFolder(ctx context.Context, path string) error
    Upload(ctx context.Context, path string, fileName string, content io.Reader, metadata FileMetadata) (StoredFile, error)
    List(ctx context.Context, path string) ([]StoredFile, error)
    Read(ctx context.Context, path string) (io.ReadCloser, error)
    Delete(ctx context.Context, path string) error
    Move(ctx context.Context, fromPath string, toPath string) error
    Rename(ctx context.Context, path string, newName string) (StoredFile, error)
}
```

A assinatura exata deve respeitar os padrões do projeto, mas precisa cobrir as operações exigidas.

## Escopo obrigatório

### 2.1 Mapear pontos atuais de dependência do Google Drive

Auditar o código e documentação para localizar:

- imports de cliente Google Drive;
- serviços de upload/download;
- helpers de criação de pastas;
- paths remotos salvos em eventos/projeções;
- rotas que leem, baixam, movem ou deletam arquivos;
- testes acoplados a Google Drive;
- variáveis de ambiente e documentação de configuração.

### 2.2 Introduzir interface comum

Criar uma interface de storage em pacote apropriado, com tipos próprios do domínio/aplicação para:

- caminho remoto;
- nome de arquivo;
- MIME type;
- tamanho;
- ID remoto opcional;
- metadados necessários para auditoria;
- erro normalizado de arquivo/pasta não encontrada.

A interface deve evitar expor tipos específicos do Mega ou Google Drive.

### 2.3 Implementar adapter Mega

Implementar um adapter concreto para Mega que satisfaça a interface comum e encapsule:

1. autenticação;
2. resolução de diretórios;
3. criação idempotente de pastas;
4. upload;
5. listagem/leitura;
6. deleção;
7. movimentação;
8. renomeação;
9. normalização de erros.

### 2.4 Remover uso direto do Google Drive no fluxo principal

Substituir chamadas diretas ao Google Drive pelo uso da interface. Ao final, o fluxo principal de arquivos deve usar o provider Mega.

Se for necessário manter algum código antigo temporariamente para referência, ele não pode ser usado em runtime, não pode ser selecionado por padrão e deve estar claramente isolado. Preferir remover dependências não utilizadas.

---

# 3. Gerenciar localização, diretórios e movimentação de arquivos

## Objetivo

Garantir que o backend consiga controlar a localização dos arquivos e pastas no Mega, incluindo mudança de diretórios e movimentação de arquivos entre pastas sem quebrar referências internas.

## Regra de negócio

O sistema deve continuar representando a localização dos documentos por caminhos remotos estáveis e compreensíveis, preservando as estruturas já definidas para academias, estudantes, solicitações de matrícula e documentação formal.

Ao mover um arquivo ou pasta, o backend deve:

1. validar origem existente;
2. garantir destino existente ou criá-lo quando a regra permitir;
3. mover o arquivo no Mega;
4. atualizar a referência persistida/eventual correspondente;
5. registrar evento/log/auditoria conforme padrão existente;
6. evitar deixar arquivo duplicado ou órfão em falhas parciais;
7. retornar erro padronizado quando origem/destino for inválido.

## Escopo obrigatório

### 3.1 Padronizar resolução de paths

Centralizar helpers de construção de paths para evitar concatenações manuais espalhadas no sistema. Exemplos de estruturas que devem continuar funcionando:

```text
{codigo_academia}/Documentação formal/
{codigo_academia}/Estudantes/{codigo_estudante}/
{codigo_academia}/Solicitações de matrícula/{codigo_solicitacao}/
```

Os nomes finais devem respeitar o padrão real já adotado pelo projeto.

### 3.2 Criar operações de movimentação

Adicionar suporte de aplicação/infraestrutura para:

- mover arquivo entre diretórios;
- mover diretório quando necessário;
- renomear arquivo;
- trocar documento de pasta mantendo metadados;
- atualizar projeções/referências após a movimentação.

### 3.3 Tratar consistência em falhas

Definir estratégia para falhas parciais entre storage externo e persistência/event sourcing. A implementação deve minimizar estados inconsistentes com uma das abordagens aceitas pelo projeto:

- executar upload/move antes do evento e gravar evento apenas após sucesso;
- usar job de compensação para limpar arquivo órfão;
- registrar estado intermediário auditável e reconciliável;
- ou outro padrão já existente no backend.

Documentar a decisão quando houver trade-off relevante.

---

# 4. Preservar contratos e fluxos existentes

## Objetivo

Trocar o provedor de armazenamento sem alterar a grande estrutura do sistema nem exigir mudanças desnecessárias de API para clientes externos.

## Regras obrigatórias

A alteração não deve:

1. mudar contratos públicos de endpoints sem necessidade explícita;
2. alterar nomes de campos de documentos já persistidos;
3. quebrar eventos existentes sem migração/compatibilidade planejada;
4. reescrever aggregates por causa do provedor;
5. expor IDs internos do Mega em contratos onde antes eram usados paths lógicos;
6. espalhar lógica de Mega em handlers ou domínio;
7. depender de Google Drive para novos uploads, deleções, listagens ou movimentações.

## Escopo obrigatório

### 4.1 Atualizar serviços de documentos existentes

Revisar todos os fluxos que manipulam arquivos, incluindo quando existirem:

- cadastro de academia e documentos formais;
- documentos de estudantes;
- solicitação de matrícula;
- aprovação/reprovação com deleção de documentos;
- downloads administrativos;
- jobs assíncronos com anexos;
- importações/exportações;
- qualquer validação de PDF acoplada ao provedor.

### 4.2 Compatibilizar referências persistidas

Não há necessidade de migração de arquivos do Google Drive para o Mega, pois não existe conteúdo armazenado no Google Drive atualmente. Portanto, a atualização deve tratar o Google Drive apenas como dependência operacional a ser removida/inativada, sem planejar cópia, sincronização, leitura legado ou migração automática de arquivos.

Se, durante a implementação, forem encontrados caminhos ou referências persistidas ao Google Drive, eles devem ser avaliados como metadados legados sem arquivo remoto correspondente e tratados conforme o padrão de compatibilidade do projeto, sem criar uma etapa de migração de arquivos. Para novos dados, o provedor deve ser Mega.

### 4.3 Atualizar erros padronizados

Todos os erros vindos do Mega devem ser convertidos para o padrão de erro mais recente do backend, incluindo:

- credenciais inválidas;
- pasta não encontrada;
- arquivo não encontrado;
- arquivo duplicado quando aplicável;
- quota excedida;
- timeout/falha de rede;
- indisponibilidade temporária do provedor;
- operação não suportada pela biblioteca.

---

# 5. Atualizar testes

## Objetivo

Garantir que a troca de provider seja segura e que a nova abstração possa ser testada sem depender de uma conta Mega real em testes unitários.

## Escopo obrigatório

Adicionar ou ajustar testes cobrindo:

1. seleção do provider Mega por configuração;
2. erro de configuração quando credenciais Mega estiverem ausentes;
3. upload usando a interface comum;
4. criação idempotente de diretório;
5. listagem/leitura de arquivos;
6. deleção de arquivo existente;
7. deleção de arquivo inexistente retornando erro normalizado;
8. movimentação de arquivo entre diretórios;
9. atualização de referência após movimentação;
10. renomeação quando suportada;
11. conversão de erros da lib Mega para erros padronizados;
12. ausência de dependência direta do Mega em handlers/domínio;
13. fluxos existentes de documentos continuando com os mesmos contratos de API;
14. testes com fake/mock de `FileStorage` para casos de uso;
15. testes de integração opcionais com Mega real protegidos por build tag, variável de ambiente ou suite separada.

## Regras para testes de integração

Testes que usam conta Mega real devem:

- ser opt-in;
- nunca rodar por padrão em CI sem credenciais explícitas;
- usar pasta temporária isolada;
- limpar arquivos criados ao final;
- não logar credenciais;
- tolerar indisponibilidade externa sem quebrar a suite unitária principal.

---

# 6. Atualizar documentação e configuração operacional

## Objetivo

Remover instruções operacionais centradas no Google Drive e documentar o novo modelo Mega/provider.

## Escopo obrigatório

Atualizar, quando aplicável:

1. README e documentação de ambiente;
2. documentação de configuração de storage;
3. guias de deploy;
4. OpenAPI/Swagger se houver endpoints de download/listagem impactados;
5. exemplos de `.env`;
6. documentação técnica sobre diretórios remotos;
7. runbook de problemas comuns de storage.

A documentação deve explicar:

- variáveis necessárias para Mega;
- como validar credenciais;
- como escolher provider por configuração;
- como rodar testes unitários e integração;
- limitações conhecidas da biblioteca escolhida;
- ausência de necessidade de migração, pois não há arquivos armazenados no Google Drive atualmente.

---

# 7. Critérios de aceite

A atualização será considerada concluída quando:

1. nenhum fluxo principal de arquivo depender operacionalmente do Google Drive;
2. o Mega estiver configurado como provider principal de storage;
3. a autenticação Mega funcionar com e-mail e senha;
4. o sistema conseguir criar/localizar pastas, fazer upload, listar/ler, deletar e mover arquivos;
5. a camada de domínio/handlers depender apenas da interface comum de storage;
6. as referências internas de arquivos continuarem estáveis e coerentes;
7. a mudança de provider no futuro exigir troca/criação de adapter, não refatoração ampla do sistema;
8. testes unitários cobrirem casos de sucesso e erro com fake/mock;
9. testes de integração Mega real forem opcionais e isolados;
10. documentação operacional estiver atualizada;
11. variáveis e referências ativas ao Google Drive forem removidas ou explicitamente marcadas como legado inativo;
12. erros retornados ao cliente seguirem o padrão mais recente do backend.

## Fora de escopo

Esta tarefa não exige:

- criar UI administrativa para navegar arquivos;
- alterar regras de obrigatoriedade de documentos;
- alterar limite de tamanho/MIME type de PDFs, salvo se houver acoplamento com o provider antigo;
- migrar arquivos antigos do Google Drive para o Mega, pois não há arquivos armazenados no Google Drive atualmente;
- expor IDs internos do Mega para clientes externos;
- reestruturar módulos de negócio que apenas consomem storage.
