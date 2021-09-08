+++
title = "The Rust Borrow Checker"
date = "2021-09-07"
+++

I’ve been having fun with Rust lately.

Rust is notoriously difficult, but at some point it clicks and starts to look like any language: structs and methods. Except you don’t have to worry about a bad `free()` causing a vulnerability, or basic string operation segfaulting.

Success with Rust’s [memory model][rs-ownership] depends on understanding a few core concepts, and this post will go over references (and when to avoid them).

My first mistake was to think of references (`&T`) as pointers. While they’re related, there’s many cases where you’d use a pointer in other languages but shouldn’t use a reference. Rust even has a non-reference type ([Box][rs-box]) that represents a pointer.

If you don’t have a local Rust install, everything in this post runs in the [Rust Playground][rs-play].

[rs-box]: https://doc.rust-lang.org/std/boxed/struct.Box.html
[rs-ownership]: https://doc.rust-lang.org/book/ch04-00-understanding-ownership.html
[rs-play]: https://play.rust-lang.org/
## Owned variables
Regular variables in Rust are “owned.” They’re single-assignment, and if reassigned or used as an argument, they “move” and can’t be used after. In this case, once we call `say_hello`, we transfer ownership of `g` to that function, and can’t reference it again.

```
struct Greeter {
    name: String,
}

impl Greeter {
    fn new() -> Self {
        Self {
            name: "Rust".to_string(),
        }
    }
    fn greeting(self) -> String {
        format!("Hello, {}!", self.name)
    }
}

fn say_hello(g: Greeter) {
    println!("{}", g.greeting());
}

fn main() {
    // "g" is an "owned" value.
    let g = Greeter::new();
    say_hello(g); // "g" is moved here. Can no longer be accessed.
}
```

When composing structs or returning values, default to owned variables rather than references. You can take references later, but can’t turn a referenced variable into an owned one.

```
use std::fs;
use std::net;

// This code uses owned variables, rather than references.
fn new_session() -> Session {
	Session { /* ... */ }
}
struct Session {
    // ...
}
struct Logger {
    out: fs::File,
}
struct Server {
    stream: net::TcpStream,
    sessions: Vec<Session>,
    logger: Logger,
}
```
## References
Instead of moving a variable, we can let functions “borrow” it with a reference. Unlike C, `&T` is the syntax for creating a reference and a reference type. Functions that receive a reference use `fn foo(&T)` not `fn foo(*T)`.

We’ll change the `greeting()` function to take a reference, and can then call `say_hello()` without moving the variable.

```
​​struct Greeter {
    name: String,
}

impl Greeter {
    fn new() -> Self {
        Self {
            name: "Rust".to_string(),
        }
    }
    fn greeting(&self) -> String {
        format!("Hello, {}!", self.name)
    }
}

fn say_hello(g: &Greeter) {
    println!("{}", g.greeting());
}

fn main() {
    // "g" is an "owned" value.
    let g = Greeter::new();
    say_hello(&g); // No move, "g" is now borrowed.
    say_hello(&g)
}
```

Variables and references are immutable by default. To make them mutable, they must be annotated as `mut T`. The calling code doesn’t change, but because of the `&mut self` receiver, `set_name()` can now modify struct fields on `Greeter`.

```
struct Greeter {
    name: String,
}

impl Greeter {
    fn new() -> Self {
        Self {
            name: "Rust".to_string(),
        }
    }
    fn greeting(&self) -> String {
        format!("Hello, {}!", self.name)
    }
    fn set_name(&mut self, name: String) {
        self.name = name;
    }
}

fn main() {
    // "g" is an "owned" value.
    let mut g = Greeter::new();
    println!("{}", g.greeting());
    g.set_name("Borrower".to_string());
    println!("{}", g.greeting());
}
```
## Concurrency
Rust shines in concurrent programs, where the compiler guarantees safety based on references and mutability.

Because `greeting()` takes an immutable reference, Rust knows threads can call the function simultaneously without synchronizing.

```
use lazy_static::lazy_static;
use std::thread;

lazy_static! {
    // Global "Greeter" instance.
    static ref GREETER: Greeter = Greeter::new();
}

struct Greeter {
    name: String,
}

impl Greeter {
    fn new() -> Self {
        Self {
            name: "Rust".to_string(),
        }
    }
    fn greeting(&self) -> String {
        format!("Hello, {}!", self.name)
    }
}

fn main() {
    let mut threads = Vec::new();
    for _ in 0..10 {
        threads.push(thread::spawn(move || {
            println!("{}", GREETER.greeting()); // No locking required.
        }));
    }
    for t in threads {
        t.join().unwrap();
    }
}
```

Only one thread can hold a mutable reference at once, so for multiple threads to `set_name()`, we’ll need a locking primitive. In this case, we’ll use a mutex and lock in each thread.

```
use lazy_static::lazy_static;
use std::sync;
use std::thread;

lazy_static! {
    // Global "Greeter" instance guarded by a mutex.
    static ref GREETER: sync::Mutex<Greeter> = sync::Mutex::new(Greeter::new());
}

struct Greeter {
    name: String,
}

impl Greeter {
    fn new() -> Self {
        Self {
            name: "Rust".to_string(),
        }
    }
    fn set_name(&mut self, name: String) {
        self.name = name
    }
    fn greeting(&self) -> String {
        format!("Hello, {}!", self.name)
    }
}

fn main() {
    let mut threads = Vec::new();
    for i in 0..10 {
        threads.push(thread::spawn(move || {
            let mut g = GREETER.lock().unwrap();
            g.set_name(format!("Thread #{}", i + 1));
            println!("{}", g.greeting());
        }));
    }
    for t in threads {
        t.join().unwrap();
    }
}
```

That’s an awesome thing about Rust. Even with threaded code, if it compiles, it’s memory safe.
