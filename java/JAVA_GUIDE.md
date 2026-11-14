# Java / Spring Boot Guide (for Go and Node developers)

A walkthrough of the concepts used in this project, mapped to Go and Node.js equivalents.

## Project shape

- `pom.xml` is Maven's `package.json` plus build config. The `spring-boot-starter-*` entries are curated bundles of dependencies. `mvn test` is `npm test`.
- Packages are folders (`com/glitchcraft/likecounter`), and each public class lives in its own same-named file. `import` works like Node's `require`.

## Spring Boot (think Express with a built-in DI container)

- `@SpringBootApplication` on `LikeCounterApplication` boots an embedded server (Tomcat) and scans for components. There is no `app.listen`.
- `@RestController` plus `@GetMapping` / `@PostMapping` is Express's `app.get(...)` routes, declared with annotations on methods.
- `@PathVariable` is `req.params`, and `@RequestBody` is `req.body` parsed into a typed object. `@Valid` and `@NotBlank` do the validation written by hand in the Node version.
- `record LikeRequest(String userId)` is an immutable struct, like a Go struct with a free constructor. JSON maps onto it automatically.

## Dependency injection

- `@Service` marks a class that Spring creates once, as a singleton. The controller's constructor asks for `PostLikeService`, and Spring passes it in. In Go you wire this by hand in `main`; in Node you `require` a module singleton.
- `JdbcClient` is injected the same way. It uses the Hikari connection pool, comparable to Go's `database/sql` pool or Node's `pg.Pool`.

## Data access

- `JdbcClient` is plain SQL with named parameters (`:postId`). `.update()` returns affected rows, like Go's `RowsAffected()`, and `.query(...).optional()` returns an `Optional`.
- `Optional<T>` is Java's explicit "might be absent" wrapper, standing in for `null` or Go's `(v, ok)`.

## Transactions

- In Go you call `tx := db.Begin()` and defer `Rollback`. In Node the project has a `withTransaction` helper. In Java, `@Transactional` makes Spring wrap the method: it begins, commits on a normal return, and rolls back on an unchecked exception.
- It works through a proxy, so it only applies when the call comes from another bean (the controller calling the service). A method calling a sibling method in the same class bypasses it.

## Errors and concurrency

- `ResponseStatusException(404)` is thrown rather than returned, so there is no `if err != nil`. Spring turns it into the HTTP response, much like Express error middleware.
- In the tests, `ExecutorService` is a goroutine pool and `Future.get()` is a channel receive that rethrows errors. `CountDownLatch` is a `sync.WaitGroup` used as a start gun.
- Java threads are heavy OS threads, unlike goroutines or Node's event loop. That is why the tests use a bounded pool of 50 threads and why Hikari caps DB connections at 10.

## Testing

- `@SpringBootTest` boots the whole app context, with a real DB, inside the test, and `@Autowired` injects beans into it.
- `@BeforeEach` is Jest's `beforeEach`, and AssertJ's `assertThat(...)` is `expect(...)`.
