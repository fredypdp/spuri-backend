# Implementar períodos fixos e imutáveis dos anos letivos (feito)

## Contexto

O sistema deve tratar o período dos anos letivos como uma regra fixa de negócio, derivada exclusivamente do tipo de ensino. Esses períodos não devem ser configuráveis por academia, admin, payload de API, migração operacional ou qualquer fluxo de manutenção.

A regra oficial é:

| Tipo de ano letivo | Período fixo | Interpretação |
| --- | --- | --- |
| `escolar` | `09_07` | começa em setembro e termina em julho |
| `superior` | `10_07` | começa em outubro e termina em julho |

O identificador do ano letivo continua evolutivo, por exemplo `2025_2026`, `2026_2027`, etc. Já o período é imutável e deve ser sempre calculado/atribuído pelo backend a partir do tipo de ensino.

## Problema

Qualquer implementação que aceite, persista ou atualize o campo `periodo` como dado configurável pode gerar inconsistências acadêmicas, como:

- ano letivo escolar usando período de ensino superior;
- ano letivo superior usando período escolar;
- academias diferentes com períodos divergentes para o mesmo tipo de ensino;
- validações de faltas, matrículas, progressão ou encerramento de ano baseadas em janelas incorretas;
- payloads permitindo alterar uma regra que deve ser sistêmica.

## Objetivo

Implementar a regra de períodos fixos e imutáveis dos anos letivos no backend:

- `escolar` deve sempre usar `09_07`;
- `superior` deve sempre usar `10_07`;
- nenhum fluxo deve aceitar alteração arbitrária do período;
- qualquer valor recebido do cliente para `periodo` deve ser ignorado, rejeitado ou substituído pela regra fixa, conforme o contrato atual da API permitir com menor risco de quebra;
- respostas da API podem expor `periodo`, mas apenas como valor derivado/fixo do sistema.

## Escopo obrigatório

### 1. Centralizar a regra fixa de período

Criar ou ajustar uma função/constante única para resolver período por tipo de ano letivo.

A implementação deve evitar strings mágicas espalhadas pelo código. A regra esperada é:

```text
escolar  -> 09_07
superior -> 10_07
```

A função deve falhar de forma explícita para tipos desconhecidos, quando aplicável.

### 2. Bloquear período configurável em criação e atualização

Revisar os fluxos de criação e atualização de anos letivos para garantir que:

- o backend não aceite `periodo` como fonte de verdade enviada pelo cliente;
- `periodo` seja sempre definido a partir do tipo (`escolar` ou `superior`);
- não exista endpoint administrativo capaz de transformar escolar em `10_07` ou superior em `09_07`;
- updates não permitam trocar apenas o período preservando o mesmo tipo.

Se houver payloads antigos com `periodo`, escolher uma estratégia segura e documentada:

- rejeitar o payload com erro de validação quando o valor divergir da regra fixa; ou
- ignorar o campo e persistir o valor fixo derivado do tipo.

A decisão deve ser consistente nos handlers, services e validações.

### 3. Corrigir persistência e migrações, se necessário

Verificar se existem registros, seeds ou migrações que gravam períodos letivos configuráveis.

A implementação deve garantir que dados persistidos respeitem:

- anos letivos escolares com `periodo = '09_07'`;
- anos letivos superiores com `periodo = '10_07'`.

Se houver dados legados incompatíveis, criar migração corretiva ou orientar o ajuste de forma segura, preservando histórico quando necessário.

### 4. Atualizar validações dependentes de datas letivas

Qualquer validação que calcule intervalo real do ano letivo deve usar o período fixo do tipo, combinado com o identificador do ano letivo.

Exemplo para `ano_letivo = "2025_2026"`:

- `escolar` + `09_07`: `2025-09-01` até `2026-07-31`;
- `superior` + `10_07`: `2025-10-01` até `2026-07-31`.

Revisar, no mínimo, regras relacionadas a:

- faltas;
- matrícula/vínculo acadêmico;
- encerramento de ano letivo;
- progressão acadêmica;
- qualquer cálculo que dependa de início/fim do período letivo.

### 5. Atualizar documentação e contratos expostos

A documentação da API e das regras de negócio deve deixar claro que `periodo` é um valor fixo do sistema.

Não deve haver exemplo ou contrato que sugira payloads como:

```json
{
  "type": "escolar",
  "periodo": "10_07"
}
```

ou:

```json
{
  "type": "superior",
  "periodo": "09_07"
}
```

como operações válidas.

## Critérios de aceite

- Existe uma regra centralizada para mapear `escolar -> 09_07` e `superior -> 10_07`.
- Criação de ano letivo escolar sempre grava/retorna `09_07`.
- Criação de ano letivo superior sempre grava/retorna `10_07`.
- Atualizações não permitem alterar o período para valor incompatível com o tipo.
- Nenhum endpoint usa `periodo` enviado pelo cliente como fonte de verdade para definir a janela letiva.
- Validações de datas acadêmicas usam o período fixo derivado do tipo.
- Testes cobrem pelo menos criação, atualização/rejeição de período inválido e cálculo de intervalo real para escolar e superior.
- Documentação deixa explícito que os períodos são fixos, imutáveis e não configuráveis por academia ou admin.

## Sugestão de testes

- Teste unitário da função que resolve período por tipo.
- Teste de criação de ano letivo escolar retornando `09_07`.
- Teste de criação de ano letivo superior retornando `10_07`.
- Teste de tentativa de enviar período incompatível no payload.
- Teste de cálculo de intervalo para `2025_2026` escolar: `2025-09-01` a `2026-07-31`.
- Teste de cálculo de intervalo para `2025_2026` superior: `2025-10-01` a `2026-07-31`.
