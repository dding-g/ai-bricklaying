# ssot/domains 규칙

## 목적

- `ssot/domains`는 ai-bricklaying의 제품 책임별 SSOT 문서 세트를 관리한다.
- 현재 domain은 `cli` 하나다.

## 작성 규칙

- 새 behavior는 먼저 어느 domain 책임인지 판단한다.
- 현재는 `cli/index.md` 단일 파일이 10개 필수 축을 모두 담는다.
- `cli/index.md`가 너무 커지거나 CLI 외 domain이 생기면 `ssot/rules.md`의 번호 문서 세트로 분리한다.
- README나 test에서 발견한 behavior를 SSOT에 반영할 때는 user-facing 계약인지, implementation detail인지 먼저 구분한다.
- 구현 회고, 임시 우회, session transcript 원문은 domain SSOT에 넣지 않는다.
- `ssot/domains/AGENTS.md` 자체는 repository instruction file이며 canonical SSOT frontmatter 규칙의 예외다.

## 참조 규칙

- Domain 문서가 추가되면 `ssot/index.md`의 문서 목록과 `ref`를 함께 갱신한다.
- 공통 규칙은 `ssot/rules.md`에 둔다.

## 금지 사항

- 같은 CLI option이나 output contract를 여러 domain 문서에 반복 정의하지 않는다.
- Generated summary, generated skill, local config, private daily state/lock, confirmed worklog를 정본 문서처럼 수동 수정하지 않는다.
