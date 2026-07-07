# Documentar períodos fixos e imutáveis dos anos letivos

## Contexto

O sistema deve tratar os períodos dos anos letivos como regras fixas de negócio, e não como configurações variáveis por academia, ano ou operação administrativa.

Essa documentação deve deixar explícito que o período letivo é determinado exclusivamente pelo tipo de ensino e permanece imutável no sistema.

## Problema

Há risco de a documentação e as futuras implementações tratarem o campo `periodo` como configurável ou editável, permitindo interpretações diferentes entre ano letivo escolar e superior.

A regra correta é:

- ano letivo escolar sempre começa em setembro e termina em julho;
- ano letivo superior sempre começa em outubro e termina em julho;
- esses períodos são fixos, imutáveis e não devem ser alterados por admin, academia ou fluxo operacional.

## Objetivo

Atualizar a documentação do sistema para registrar que os períodos dos anos letivos são fixos e imutáveis:

- `escolar`: `09_07`;
- `superior`: `10_07`.

O `ano_letivo` continua sendo o identificador anual evolutivo, por exemplo `2025_2026`, `2026_2027`, etc. Já o `periodo` é uma regra estática associada ao tipo de ano letivo e não deve ser versionado, recriado ou editado a cada virada de ano.

## Escopo obrigatório

### 1. Documentar a regra de período fixo

Registrar na documentação que os períodos oficiais são:

| Tipo | Período | Interpretação |
| --- | --- | --- |
| `escolar` | `09_07` | começa em setembro do ano inicial e termina em julho do ano final |
| `superior` | `10_07` | começa em outubro do ano inicial e termina em julho do ano final |

Exemplo para `ano_letivo = "2025_2026"`:

- `escolar` + `09_07`: de `2025-09-01` até `2026-07-31`;
- `superior` + `10_07`: de `2025-10-01` até `2026-07-31`.

### 2. Remover/ajustar linguagem que sugira configuração livre

Revisar documentos que descrevam anos letivos e ajustar qualquer trecho que indique que o `periodo` pode ser configurado livremente por admin FPP, academia ou usuário operacional.

A documentação deve usar linguagem explícita como:

- "período fixo";
- "imutável";
- "definido pelo sistema";
- "não configurável por academia";
- "não editável por fluxos administrativos".

### 3. Diferenciar `ano_letivo` de `periodo`

Documentar que:

- `ano_letivo` é o ciclo anual e muda com o tempo (`2025_2026`, `2026_2027`, etc.);
- `periodo` é a janela fixa em meses associada ao tipo de ensino;
- o sistema calcula datas reais combinando `ano_letivo` + `periodo`;
- a virada de ano letivo não altera os períodos fixos.

### 4. Documentar impactos esperados em validações

A documentação deve orientar que validações de datas acadêmicas usem os períodos fixos:

- faltas escolares devem respeitar o intervalo calculado por `09_07`;
- faltas superiores devem respeitar o intervalo calculado por `10_07`;
- outras regras dependentes de período letivo devem consultar a regra fixa do tipo, e não aceitar período enviado pelo cliente.

### 5. Documentar restrições de API e payloads

Se a documentação da API expuser `periodo`, deixar claro que ele é retornado apenas como informação derivada/fixa do sistema.

Não deve haver documentação de endpoint ou payload que permita alterar:

```json
{
  "type": "escolar",
  "periodo": "10_07"
}
```

para transformar o escolar em outubro-julho, pois o valor correto e único para `escolar` é `09_07`.

Da mesma forma, não deve haver documentação permitindo transformar o superior em `09_07`; o valor correto e único para `superior` é `10_07`.

## Áreas prováveis de documentação

Confirmar no repositório antes de editar, mas revisar pelo menos:

- `docs/Documentação.md`;
- documentos em `docs/Tarefas feitas/` que descrevam separação de ano letivo escolar/superior;
- documentos em `docs/Lista de tarefas/` ou `docs/Debbugs/` que ainda citem período configurável;
- qualquer documentação de endpoints administrativos de ano letivo.

## Critérios de aceite

- A documentação afirma explicitamente que os períodos são fixos e imutáveis.
- A documentação registra `escolar = 09_07` e `superior = 10_07`.
- Não resta orientação documental dizendo que admin, academia ou usuário pode configurar livremente o período dos tipos letivos.
- A diferença entre `ano_letivo` evolutivo e `periodo` fixo está clara.
- Exemplos com `2025_2026` mostram corretamente:
  - escolar: `2025-09-01` a `2026-07-31`;
  - superior: `2025-10-01` a `2026-07-31`.
