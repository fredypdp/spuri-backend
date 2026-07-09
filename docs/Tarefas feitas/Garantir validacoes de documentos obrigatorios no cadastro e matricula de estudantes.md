---
criado: 2026-07-09 00:00
origem: solicitação do usuário
status: feito
---

# Garantir validações de documentos obrigatórios no cadastro e matrícula de estudantes

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que as validações de obrigatoriedade de documentos no cadastro direto de estudante pela academia e na solicitação de matrícula reflitam exatamente as regras de produto por ano acadêmico, nível de ensino e tipo de identificação. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade, fallbacks temporários ou exceções não documentadas.

## Contexto

As regras de matrícula e cadastro de estudantes precisam estar alinhadas ao fluxo real de admissão por nível de ensino. A obrigatoriedade de declaração, certificado, BI do estudante, BI do responsável e cédula varia conforme o ano acadêmico pretendido e conforme o estudante esteja no nível escolar ou no ensino superior.

A atualização deve garantir que as mesmas validações sejam aplicadas tanto na solicitação de matrícula feita pelo estudante/responsável quanto no cadastro direto realizado pela academia. A validação deve considerar simultaneamente os arquivos enviados e os campos do request, impedindo inconsistências entre documentação anexada e dados declarados.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| `1_ano_fundamental` | Não cobrar declaração nem certificado | Permitir cadastro/matrícula sem comprovativo acadêmico anterior |
| `7_ano_fundamental` | Exigir certificado do `6_ano_fundamental` ou declaração desse mesmo ano acadêmico | Aceitar um dos dois documentos válidos |
| `1_ano_medio` | Exigir certificado do `9_ano_fundamental` ou declaração desse mesmo ano acadêmico | Aceitar um dos dois documentos válidos |
| `1_ano_superior` | Exigir certificado do ensino médio ou declaração desse mesmo ano acadêmico | Aceitar um dos dois documentos válidos |
| BI do estudante no superior | Obrigatório no documento e no campo do request | Responsável fica opcional no ensino superior |
| BI do responsável no escolar | Sempre obrigatório | Estudante deve informar BI próprio ou cédula |
| BI/cédula do estudante no escolar | Exigir BI do estudante ou cédula | Quando BI for usado, validar documento e campo do request |
| Fluxos afetados | Solicitação de matrícula e cadastro direto pela academia | Mesmas regras em ambos os caminhos |

---

# 1. Padronizar validação de comprovativo acadêmico por ano acadêmico

## Objetivo

Garantir que a cobrança de declaração ou certificado seja feita somente nos anos acadêmicos em que a regra de negócio exige comprovativo acadêmico anterior ou equivalente.

## Regra de negócio

Ao criar uma solicitação de matrícula ou cadastrar diretamente um estudante pela academia, o backend deve:

1. identificar o ano acadêmico pretendido do estudante;
2. aplicar a regra de documentos acadêmicos obrigatórios conforme o ano acadêmico;
3. aceitar certificado ou declaração apenas quando a regra permitir alternativa;
4. validar que o documento enviado corresponde ao ano acadêmico exigido;
5. retornar erro padronizado quando o documento obrigatório estiver ausente, inválido ou incompatível;
6. aplicar exatamente a mesma regra nos dois fluxos: solicitação de matrícula e cadastro direto pela academia.

## Escopo obrigatório

### 1.1 `1_ano_fundamental`

Para matrícula ou cadastro no `1_ano_fundamental`:

1. não exigir declaração;
2. não exigir certificado;
3. não bloquear a criação por ausência de comprovativo acadêmico anterior;
4. manter apenas as validações de identificação e demais documentos aplicáveis ao nível escolar.

### 1.2 `7_ano_fundamental`

Para matrícula ou cadastro no `7_ano_fundamental`:

1. exigir certificado do `6_ano_fundamental`; ou
2. aceitar, como alternativa, declaração referente ao mesmo ano acadêmico exigido;
3. rejeitar certificado ou declaração de outro ano acadêmico;
4. rejeitar a operação quando nenhum dos dois documentos válidos for enviado.

### 1.3 `1_ano_medio`

Para matrícula ou cadastro no `1_ano_medio`:

1. exigir certificado do `9_ano_fundamental`; ou
2. aceitar, como alternativa, declaração referente ao mesmo ano acadêmico exigido;
3. rejeitar certificado ou declaração de outro ano acadêmico;
4. rejeitar a operação quando nenhum dos dois documentos válidos for enviado.

### 1.4 `1_ano_superior`

Para matrícula ou cadastro no `1_ano_superior`:

1. exigir certificado do ensino médio; ou
2. aceitar, como alternativa, declaração referente ao mesmo ano acadêmico exigido;
3. rejeitar certificado ou declaração incompatível com conclusão/declaração do ensino médio;
4. rejeitar a operação quando nenhum dos dois documentos válidos for enviado.

### 1.5 Demais anos acadêmicos

Auditar os demais anos acadêmicos existentes para garantir que não haja cobrança indevida de declaração ou certificado fora das regras documentadas nesta tarefa, salvo regra já existente e explicitamente mantida em documentação ativa.

---

# 2. Validar BI do estudante e do responsável no ensino superior

## Objetivo

Garantir que, no ensino superior, a identificação obrigatória seja a do próprio estudante, enquanto a identificação do responsável permaneça opcional.

## Regra de negócio

Para estudantes do ensino superior, incluindo `1_ano_superior`, o backend deve:

1. exigir BI do estudante como documento anexado;
2. exigir o campo de BI do estudante no request;
3. validar consistência entre o campo informado e o documento enviado, quando houver regra de consistência no projeto;
4. não exigir BI do responsável;
5. tratar dados e documentos do responsável como opcionais;
6. aplicar a regra tanto em solicitação de matrícula quanto em cadastro direto pela academia.

## Escopo obrigatório

### 2.1 Campo do request

Atualizar DTOs, validators, schemas, commands, handlers e documentação para que o campo de BI do estudante seja obrigatório no ensino superior.

O campo de BI do responsável não deve ser obrigatório para estudantes do ensino superior.

### 2.2 Documento anexado

Atualizar a validação de uploads/documentos para exigir o documento de BI do estudante no ensino superior.

O documento de BI do responsável deve ser aceito quando enviado, mas não deve bloquear a matrícula/cadastro quando ausente.

### 2.3 Testes

Adicionar ou ajustar testes cobrindo:

1. ensino superior aceitando cadastro/matrícula com BI do estudante no campo e no documento;
2. ensino superior rejeitando ausência do campo de BI do estudante;
3. ensino superior rejeitando ausência do documento de BI do estudante;
4. ensino superior aceitando ausência de BI do responsável no campo do request;
5. ensino superior aceitando ausência de documento de BI do responsável;
6. ensino superior aceitando BI do responsável quando enviado opcionalmente.

---

# 3. Validar BI do responsável, BI do estudante ou cédula no nível escolar

## Objetivo

Garantir que, no nível escolar, o responsável esteja sempre identificado por BI e que o estudante seja identificado por BI próprio ou por cédula.

## Regra de negócio

Para estudantes do nível escolar, o backend deve:

1. exigir BI do responsável no campo do request;
2. exigir documento de BI do responsável;
3. exigir, para o estudante, uma das seguintes alternativas:
   - BI do estudante no campo do request e documento de BI do estudante; ou
   - cédula do estudante conforme contrato vigente;
4. rejeitar a operação quando o estudante não possuir nem BI completo nem cédula válida;
5. aplicar a regra tanto em solicitação de matrícula quanto em cadastro direto pela academia.

## Escopo obrigatório

### 3.1 Responsável obrigatório

Atualizar DTOs, validators, schemas, commands, handlers e documentação para garantir que, no nível escolar, o BI do responsável seja sempre obrigatório no request e como documento anexado.

Essa regra deve valer independentemente do ano escolar pretendido.

### 3.2 Alternativa de identificação do estudante

Atualizar a validação do estudante escolar para aceitar uma das alternativas válidas:

1. BI do estudante informado no request e documento de BI do estudante anexado; ou
2. cédula do estudante informada/anexada conforme o contrato vigente do sistema.

Quando o estudante escolar usar BI, a obrigatoriedade deve existir nos dois pontos: campo do request e documento anexado.

### 3.3 Testes

Adicionar ou ajustar testes cobrindo:

1. nível escolar rejeitando ausência do campo de BI do responsável;
2. nível escolar rejeitando ausência do documento de BI do responsável;
3. nível escolar aceitando estudante com BI no campo e documento de BI anexado;
4. nível escolar rejeitando estudante com BI no campo, mas sem documento de BI, quando não houver cédula;
5. nível escolar rejeitando estudante com documento de BI, mas sem campo de BI, quando não houver cédula;
6. nível escolar aceitando estudante com cédula válida e sem BI próprio;
7. nível escolar rejeitando estudante sem BI próprio completo e sem cédula válida.

---

# 4. Unificar os fluxos de solicitação de matrícula e cadastro direto pela academia

## Objetivo

Evitar divergência entre a solicitação de matrícula e o cadastro direto de estudante feito pela academia.

## Regra de negócio

As validações de documentos obrigatórios devem ser equivalentes nos dois fluxos. A implementação deve evitar regras duplicadas e divergentes sempre que possível.

## Escopo obrigatório

### 4.1 Centralizar ou compartilhar regras

Auditar os validadores usados por:

- solicitação de matrícula;
- aprovação/conversão de solicitação em estudante, quando aplicável;
- cadastro direto de estudante pela academia;
- atualização de documentos durante matrícula/cadastro, quando aplicável.

Preferir serviço, helper, policy ou camada de domínio compartilhada para determinar quais documentos e campos são obrigatórios por nível de ensino e ano acadêmico.

### 4.2 Garantir mensagens de erro consistentes

As mensagens e códigos de erro devem indicar claramente:

1. qual documento está ausente;
2. qual campo do request está ausente;
3. qual alternativa documental seria aceita;
4. quando o documento enviado pertence a ano acadêmico incompatível;
5. o fluxo em que a validação falhou, sem expor detalhes sensíveis.

### 4.3 Atualizar contratos e documentação

Atualizar OpenAPI/Swagger, documentação técnica e exemplos de payload para demonstrar as combinações válidas de documentos e campos para cada cenário.

---

# 5. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir as regras reais de obrigatoriedade documental no cadastro e na matrícula de estudantes.

## Escopo de documentação

Atualizar, quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio de estudantes;
- documentação de solicitações de matrícula;
- documentação de cadastro direto pela academia;
- documentação de uploads e storage;
- exemplos de payload;
- coleções de API;
- documentos de tarefas anteriores usados como referência ativa.

## Regras de documentação

A documentação deve declarar explicitamente que:

- `1_ano_fundamental` não exige declaração nem certificado;
- `7_ano_fundamental` exige certificado do `6_ano_fundamental` ou declaração desse mesmo ano acadêmico;
- `1_ano_medio` exige certificado do `9_ano_fundamental` ou declaração desse mesmo ano acadêmico;
- `1_ano_superior` exige certificado do ensino médio ou declaração desse mesmo ano acadêmico;
- no ensino superior, BI do estudante é obrigatório no campo do request e como documento;
- no ensino superior, BI do responsável é opcional no campo do request e como documento;
- no nível escolar, BI do responsável é sempre obrigatório no campo do request e como documento;
- no nível escolar, o estudante deve apresentar BI próprio no campo do request e como documento, ou cédula válida;
- as mesmas regras valem para solicitação de matrícula e cadastro direto pela academia.

---

# 6. Fora de escopo

- Cobrar declaração ou certificado para `1_ano_fundamental`.
- Aceitar comprovativo acadêmico de ano diferente do exigido para `7_ano_fundamental`, `1_ano_medio` ou `1_ano_superior`.
- Tornar BI do responsável obrigatório no ensino superior.
- Tornar BI do estudante opcional no ensino superior.
- Permitir estudante escolar sem BI próprio completo e sem cédula válida.
- Permitir estudante escolar sem BI obrigatório do responsável.
- Criar regras diferentes entre solicitação de matrícula e cadastro direto pela academia.
- Criar aliases, wrappers de compatibilidade, fallbacks temporários ou flags para reativar validações antigas.
- Alterar regras de negócio não relacionadas à obrigatoriedade documental de estudantes, responsáveis, cadastro ou matrícula.

---

# 7. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `1_ano_fundamental` não cobrar declaração nem certificado;
2. `7_ano_fundamental` exigir certificado do `6_ano_fundamental` ou declaração desse mesmo ano acadêmico;
3. `1_ano_medio` exigir certificado do `9_ano_fundamental` ou declaração desse mesmo ano acadêmico;
4. `1_ano_superior` exigir certificado do ensino médio ou declaração desse mesmo ano acadêmico;
5. ensino superior exigir BI do estudante no campo do request e como documento;
6. ensino superior manter BI do responsável opcional no campo do request e como documento;
7. nível escolar exigir BI do responsável no campo do request e como documento;
8. nível escolar aceitar estudante com BI próprio completo ou cédula válida;
9. nível escolar rejeitar estudante sem BI próprio completo e sem cédula válida;
10. solicitação de matrícula e cadastro direto pela academia aplicarem exatamente as mesmas regras;
11. erros de validação seguirem o padrão atual do backend;
12. OpenAPI/Swagger, documentação técnica e exemplos estiverem atualizados;
13. testes automatizados cobrirem os cenários de comprovativo acadêmico, BI do estudante, BI do responsável, cédula e equivalência entre fluxos;
14. o PR explicar claramente as mudanças de contrato, validação documental e impacto nos fluxos de matrícula/cadastro.
