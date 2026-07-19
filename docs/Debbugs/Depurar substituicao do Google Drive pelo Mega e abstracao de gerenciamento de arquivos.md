---
modificado: 2026-07-10 00:00
criado: 2026-07-10 00:00
---
# Depurar substituição do Google Drive pelo Mega e abstração de gerenciamento de arquivos

Tarefa: [[Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos]]

## Objetivo da auditoria

Fazer uma auditoria crítica, extremamente profunda, completa e arquivo por arquivo da implementação da tarefa:

`docs/Lista de tarefas/Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **terminar a implementação e corrigir o que estiver errado**.

Esta funcionalidade é crítica porque altera o provedor externo encarregado por todos os arquivos do backend, incluindo documentos formais, documentos de estudantes, solicitações de matrícula, downloads consumidos pelo front end, deleções e movimentações. O sistema não pode continuar dependendo operacionalmente do Google Drive nos fluxos principais, não pode espalhar detalhes do Mega pelo domínio/handlers e não pode quebrar contratos públicos ou referências internas existentes.

## Resultado esperado da depuração

A depuração só pode ser encerrada quando estiver garantido que:

- o Mega é o provider principal de armazenamento de arquivos quando a configuração ativa indicar o novo provedor;
- a autenticação do Mega usa e-mail e senha por variáveis de ambiente/segredos, sem credenciais hardcoded e sem vazamento em logs, respostas ou erros;
- a biblioteca Mega escolhida suporta, direta ou indiretamente, criação/listagem de pastas, upload, leitura/download/stream, deleção, movimentação e renomeação;
- a decisão da biblioteca, suas limitações conhecidas e a forma de testar a integração estão documentadas;
- existe uma interface/abstração comum de storage, com tipos próprios do projeto, que não expõe tipos do Mega nem do Google Drive;
- handlers, domínio, aggregates, casos de uso, projeções e contratos públicos não importam bibliotecas do Mega nem do Google Drive diretamente;
- o adapter Mega encapsula autenticação, resolução de diretórios, criação idempotente de pastas, upload, listagem/leitura, download/stream, deleção, movimentação, renomeação e normalização de erros;
- nenhum fluxo principal de upload, listagem, leitura/download, deleção, movimentação ou organização de arquivos depende operacionalmente do Google Drive;
- referências ativas ao Google Drive foram removidas, substituídas ou claramente isoladas como legado inativo, sem serem selecionadas por padrão em runtime;
- os caminhos remotos lógicos já usados pelo negócio continuam estáveis e compreensíveis;
- movimentações e renomeações atualizam referências persistidas/projeções/eventos conforme o padrão real do backend;
- falhas parciais entre storage externo e persistência são tratadas com ordem segura, rollback, compensação, reconciliação ou outro padrão documentado do projeto;
- downloads ou fluxos de leitura atendem os endpoints consumidos pelo front end para baixar documentos;
- erros do Mega são convertidos para o padrão de erro mais recente do backend;
- testes unitários usam fake/mock da interface de storage e não exigem conta Mega real;
- testes de integração com Mega real, se existirem, são opt-in, isolados, limpam seus arquivos temporários e não rodam por padrão sem credenciais explícitas;
- documentação operacional, exemplos de ambiente, guias técnicos, README/OpenAPI/Swagger afetados e runbooks refletem o novo provider Mega;
- está documentado que não é necessária migração de arquivos do Google Drive para o Mega porque não há conteúdo armazenado no Google Drive atualmente;
- a tarefa original receba o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e seja movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, corrigido, testado e documentado.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente às regras abaixo:

| Área | Regra obrigatória | Proibição explícita |
| --- | --- | --- |
| Provedor | Mega deve ser o provider principal configurável para storage de arquivos | Manter Google Drive como dependência operacional do fluxo principal ou provider padrão |
| Autenticação | Usar e-mail e senha da conta Mega via configuração segura | Hardcodar credenciais, logar senha/token ou exigir autenticação manual em runtime |
| Arquitetura | Domínio, handlers e casos de uso dependem de interface comum de storage | Importar Mega/Google Drive fora de infraestrutura/adapters |
| Operações | Suportar pastas, upload, listagem, leitura/download/stream, deleção, movimentação e renomeação | Implementação parcial que só cobre upload ou que ignore download/move/delete |
| Contratos | Preservar contratos públicos, paths lógicos e regras de negócio existentes | Expor IDs internos do Mega sem necessidade ou alterar APIs por causa do provider |
| Erros | Normalizar erros externos para o padrão vigente do backend | Retornar erros crus da biblioteca Mega ou mensagens com dados sensíveis |
| Testes | Cobrir provider, adapter, fake/mock e fluxos existentes sem depender de Mega real | Testes unitários que exigem conta externa ou CI quebrando por falta de credenciais |
| Documentação | Atualizar configuração, operação, ausência de migração e limitações da biblioteca | Manter guias ativos ensinando Google Drive como storage vigente |
| Conclusão | Adicionar `(feito)` no título interno da tarefa e mover para `docs/Tarefas feitas/` | Alterar o nome do arquivo para adicionar `(feito)` ou mover antes de validar tudo |

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. configuração de ambiente, carregamento de variáveis, validação de settings e exemplos `.env`;
2. dependências do projeto, lockfiles, imports e clients externos relacionados a Mega, Google Drive, Google APIs, OAuth, service account e storage;
3. serviços/adapters/gateways de arquivos, upload, download, leitura, listagem, deleção, movimentação, renomeação e criação de pastas;
4. interfaces de aplicação/domínio usadas por handlers, casos de uso, aggregates, projections, jobs e serviços de documentos;
5. rotas/endpoints que recebem arquivos, retornam downloads, leem documentos, listam documentos, removem arquivos ou movem documentos entre pastas;
6. fluxos de academia, documentação formal, estudantes, responsáveis, solicitações de matrícula, aprovação/reprovação, importações/exportações e jobs assíncronos que tocam arquivos;
7. persistência de referências de arquivo, paths remotos, IDs remotos opcionais, metadados, eventos, snapshots, projeções e serializers;
8. tratamento de erros de autenticação, autorização, arquivo/pasta inexistente, duplicidade, quota, rate limit, timeout, rede e operação não suportada;
9. logs, auditoria, mensagens de erro e sanitização de dados sensíveis;
10. testes unitários, testes de handler, testes de serviço, testes de integração, fakes, mocks, fixtures e factories relacionados a storage;
11. documentação técnica, README, guias operacionais, OpenAPI/Swagger, runbooks e documentos históricos que ainda possam ser usados como contrato ativo;
12. scripts, seeds, jobs e ferramentas internas que façam upload/download/listagem/deleção diretamente no provedor antigo;
13. qualquer fallback, flag, modo compatível ou resíduo de Google Drive que possa ser selecionado por padrão ou por configuração comum.

## Checklist obrigatório de validação

### 1. Busca ampla e classificação de ocorrências

Fazer busca ampla no repositório por, no mínimo:

- `Google Drive`;
- `google drive`;
- `drive`;
- `googleapis`;
- `service account`;
- `service_account`;
- `GOOGLE`;
- `GDRIVE`;
- `MEGA`;
- `Mega`;
- `mega`;
- `STORAGE_PROVIDER`;
- `storage provider`;
- `FileStorage`;
- `Storage`;
- `Upload`;
- `Download`;
- `Read`;
- `Stream`;
- `List`;
- `Delete`;
- `Move`;
- `Rename`;
- `EnsureFolder`;
- `folder`;
- `path`;
- `remote`;
- `Documentação formal`;
- `Solicitações de matrícula`;
- `Estudantes`;
- nomes reais dos clients, adapters, interfaces e helpers de storage do projeto.

Não basta listar ocorrências. Cada ocorrência relevante deve ser classificada como:

- implementação correta do novo contrato;
- configuração obrigatória;
- adapter/infraestrutura correta;
- dependência indevida em camada de domínio/handler;
- documentação atualizada;
- teste cobrindo regressão;
- bug ativo a corrigir;
- resíduo legado a remover;
- código morto a apagar;
- documentação histórica aceitável apenas se não for contrato vigente.

### 2. Biblioteca Mega e dependências

Auditar a biblioteca Mega selecionada e sua integração.

Validar que:

- a biblioteca suporta autenticação por e-mail e senha;
- a biblioteca permite criar e localizar pastas;
- upload de arquivos funciona com stream, arquivo temporário ou mecanismo equivalente seguro;
- leitura/download atende o front end sem exigir exposição de credenciais ou detalhes internos do provider;
- deleção, movimentação e renomeação estão implementadas ou compostas de forma confiável;
- erros da biblioteca podem ser mapeados para erros normalizados;
- a licença e manutenção da biblioteca são aceitáveis para o projeto;
- a dependência está registrada nos manifestos/lockfiles corretos;
- não há dependências Google Drive ativas desnecessárias;
- limitações conhecidas estão documentadas, incluindo eventuais diferenças entre API oficial e biblioteca usada.

### 3. Configuração e inicialização

Auditar settings, variáveis de ambiente e bootstrap do provider.

Validar que:

- `STORAGE_PROVIDER=mega` ou o padrão equivalente seleciona o adapter Mega;
- `MEGA_EMAIL`, `MEGA_PASSWORD` e raiz/pasta base equivalente são exigidos quando Mega estiver ativo;
- ausência de configuração obrigatória falha de forma explícita e observável;
- configuração inválida não cai silenciosamente para Google Drive;
- senha/token nunca aparece em logs, traces, responses, mensagens de erro ou snapshots de configuração;
- testes conseguem instanciar fake/mock sem conexão real;
- documentação e exemplos de ambiente usam os nomes finais reais do projeto.

### 4. Abstração de storage/provider

Auditar a interface comum e os tipos envolvidos.

Validar que:

- a interface cobre criação/garantia de pastas, upload, listagem, leitura/download/stream, deleção, movimentação e renomeação;
- assinaturas usam `context` ou mecanismo equivalente quando esse for o padrão do backend;
- tipos de retorno não expõem objetos do Mega nem do Google Drive;
- erros de arquivo/pasta não encontrada, conflito, quota, permissão, timeout e indisponibilidade são normalizados;
- casos de uso e handlers recebem a interface por injeção de dependência ou mecanismo equivalente;
- fakes/mocks de storage são simples de usar em testes;
- não há acoplamento de regras de negócio ao formato interno de IDs do Mega.

### 5. Adapter Mega

Auditar a implementação concreta do Mega.

Validar que:

- autenticação é feita de forma eficiente, sem relogar desnecessariamente em cada operação quando a biblioteca permitir reaproveitamento seguro;
- criação de pastas é idempotente;
- resolução de paths é centralizada e evita duplicar diretórios por diferença de barra, acento, caixa ou espaços;
- upload grava conteúdo no local correto e retorna metadados suficientes;
- leitura/download/stream fecha recursos corretamente e não carrega arquivos grandes em memória sem necessidade;
- listagem retorna informações coerentes para arquivos e pastas;
- deleção trata inexistência de forma padronizada;
- movimentação valida origem, garante/cria destino quando permitido e não deixa duplicidade silenciosa;
- renomeação preserva metadados necessários e atualiza referência retornada;
- falhas de quota, rate limit, timeout, rede, permissão e autenticação são traduzidas;
- operações não suportadas pela biblioteca falham explicitamente, sem simular sucesso.

### 6. Remoção/inativação do Google Drive

Auditar todos os resíduos do provider antigo.

Validar que:

- novos uploads não usam Google Drive;
- downloads/listagens/deleções/movimentações do fluxo principal não usam Google Drive;
- variáveis Google Drive não são exigidas para rodar o backend com Mega;
- documentação ativa não orienta configurar Google Drive como storage vigente;
- dependências Google Drive não usadas foram removidas quando possível;
- se algum código Google Drive permanecer temporariamente, ele está isolado, não é padrão, não é chamado em runtime comum e está marcado como legado/inativo;
- não existe fallback silencioso para Google Drive em caso de erro no Mega.

### 7. Paths, diretórios e referências persistidas

Auditar a construção e persistência de caminhos.

Validar que:

- estruturas como `{codigo_academia}/Documentação formal/`, `{codigo_academia}/Estudantes/{codigo_estudante}/` e `{codigo_academia}/Solicitações de matrícula/{codigo_solicitacao}/` continuam funcionando conforme o padrão real;
- helpers de path foram centralizados ou mantidos em um único ponto coerente;
- concatenações manuais espalhadas foram corrigidas quando criarem risco de divergência;
- referências persistidas continuam usando paths lógicos estáveis quando esse é o contrato vigente;
- IDs internos do Mega só são persistidos/expostos se houver justificativa técnica e contrato claro;
- movimentações atualizam eventos, projeções, tabelas, snapshots ou metadados necessários;
- não há arquivos órfãos ou duplicados em falhas parciais previsíveis.

### 8. Fluxos de negócio com arquivos

Auditar explicitamente todos os fluxos que manipulam arquivos.

Validar, no mínimo:

- cadastro de academia e documentos formais;
- documentos de estudantes;
- documentos de responsáveis, se existirem;
- solicitações de matrícula;
- aprovação, reprovação, cancelamento ou arquivamento que removam/movam documentos;
- downloads administrativos;
- downloads consumidos pelo front end para leitura de documentos;
- importações/exportações e jobs assíncronos com anexos;
- qualquer rota que aceite PDF ou arquivo não PDF;
- permissões/autorização antes de leitura/download/deleção;
- auditoria/logs de operações relevantes.

### 9. Erros padronizados e segurança

Identificar primeiro qual é o padrão mais recente de erro do backend pela implementação real e documentação atualizada. Em seguida, validar que todos os erros de storage seguem esse padrão.

Validar erros de:

- credenciais Mega ausentes ou inválidas;
- autenticação expirada ou sessão inválida;
- permissão negada;
- pasta inexistente;
- arquivo inexistente;
- conflito/duplicidade quando aplicável;
- quota excedida;
- rate limit;
- timeout/falha de rede;
- indisponibilidade temporária do provider;
- operação não suportada;
- falha inesperada sanitizada.

Também validar que:

- respostas não incluem senha, token, cookie, headers sensíveis, stack trace externo ou IDs internos sem necessidade;
- logs têm contexto suficiente para operação e auditoria sem expor segredos;
- erros externos crus da biblioteca não vazam ao cliente.

### 10. Testes obrigatórios

Adicionar ou corrigir testes para cobrir, no mínimo:

- seleção do provider Mega por configuração;
- erro de configuração quando credenciais Mega obrigatórias estiverem ausentes;
- inicialização com fake/mock de storage sem conexão externa;
- upload usando a interface comum;
- criação idempotente de diretório;
- listagem/leitura/download de arquivos;
- deleção de arquivo existente;
- deleção de arquivo inexistente com erro normalizado;
- movimentação entre diretórios;
- renomeação;
- atualização de referência após movimentação;
- conversão de erros da biblioteca Mega para erros padronizados;
- ausência de imports Mega/Google Drive em handlers/domínio;
- fluxos existentes de documentos mantendo contratos de API;
- testes de integração com Mega real protegidos por flag/build tag/variável de ambiente, quando existirem.

### 11. Documentação operacional e de API

Auditar e atualizar documentação afetada.

Validar que:

- README, guias de ambiente e exemplos `.env` usam Mega como storage atual;
- documentação técnica explica a interface/provider e como trocar de provider no futuro;
- runbook descreve erros comuns do Mega e como diagnosticar credenciais, quota e rede;
- OpenAPI/Swagger e exemplos de endpoints de download/listagem continuam coerentes com os contratos reais;
- documentos ativos não instruem configurar Google Drive como requisito vigente;
- a ausência de migração de arquivos do Google Drive para o Mega está documentada explicitamente;
- limitações da biblioteca Mega escolhida estão registradas;
- instruções para rodar testes unitários e integrações opt-in estão claras.

## Critérios de aceite para encerrar o debbug

Só encerrar esta tarefa de depuração quando todos os itens abaixo forem verdadeiros:

1. a auditoria encontrou e classificou todas as ocorrências relevantes de Google Drive, Mega e storage;
2. qualquer lacuna encontrada foi corrigida no código, testes e documentação;
3. o backend roda com Mega como provider principal sem exigir Google Drive;
4. os fluxos principais de arquivo foram validados com testes automatizados ou justificativa técnica documentada quando algum teste não for viável;
5. testes unitários não dependem de conta Mega real;
6. integrações reais com Mega, se existirem, são opt-in e seguras;
7. contratos públicos e paths lógicos foram preservados;
8. erros seguem o padrão mais recente do backend;
9. documentação ativa foi atualizada e documentação histórica não conflita com o contrato vigente;
10. não há credenciais, tokens ou dados sensíveis hardcoded;
11. a tarefa original foi atualizada com o sufixo `(feito)` no título interno do Markdown e movida para `docs/Tarefas feitas/`, mantendo o nome do arquivo sem o sufixo `(feito)`.

## Procedimento obrigatório de finalização da tarefa original

Depois de confirmar que a implementação está correta ou depois de terminar todas as correções necessárias:

1. abrir `docs/Lista de tarefas/Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos.md`;
2. alterar apenas o título interno de `# Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos` para `# Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos (feito)`;
3. não adicionar `(feito)` ao nome do arquivo;
4. mover o arquivo para `docs/Tarefas feitas/Substituir Google Drive pelo Mega e abstrair gerenciamento de arquivos.md`;
5. revisar links/wiki links afetados, se houver;
6. somente então considerar a auditoria concluída.

## Comandos mínimos sugeridos

Adaptar os comandos ao runtime real do projeto, mas executar uma bateria equivalente a:

```bash
rg -n "Google Drive|google drive|googleapis|service account|service_account|GOOGLE|GDRIVE|MEGA|Mega|mega|STORAGE_PROVIDER|FileStorage|Upload|Download|Read|Stream|List|Delete|Move|Rename|EnsureFolder" .
rg -n "Documentação formal|Documentacao formal|Solicitações de matrícula|Solicitacoes de matricula|Estudantes|storage|provider|remote|path|folder" .
```

Além das buscas, rodar os testes/lints/builds oficiais do backend e qualquer suite específica de storage. Registrar no relatório final o comando exato, resultado e limitações de ambiente.
