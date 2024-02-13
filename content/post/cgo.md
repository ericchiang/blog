+++
title = "Calling C from Go"
date = "2024-02-17"
published = "2024-02-16"
+++

What's a little shared memory between friends?

As someone who works a lot with operating systems, there are many scenario that require loading C libraries. Plugins that use [shared libraries](https://github.com/google/go-pkcs11), low-level [device APIs](https://github.com/go-piv/piv-go), random [Linux utilities](https://github.com/google/go-tspi). Despite modern options for interprocess communication, sometimes you get a header file and a shared object and have to run with it.

This post covers [cgo](https://pkg.go.dev/cmd/cgo), Go's C interoperability layer.

## Referencing C

Go programs reference C symbols through the magic "C" package. This is a pseudo-package that exposes C symbols to Go, as well as a number of utilities we'll cover later.

The "C" package allows defining C includes and configuration above the import statement. Usually reserved for `#include <...>` declarations and C compiler and linker options, the preamble can hold small bits of code to be used by Go.

```
package main

/*
int add(int a, int b) {
	return a + b;
}
*/
import "C"
import "fmt"

func main() {
	var a, b C.int = 1, 2
	n := C.add(a, b)
	fmt.Printf("1 + 2 = %d\n", n)
}
```

Any type can be referenced through the "C" import, not only exported functions. For example, a struct or enum typedef. 

```
package main

/*
typedef struct adder {
	int a;
	int b;
} adder_t;

int adder_add(adder_t *a) {
	return a->a + a->b;
}
*/
import "C"
import "fmt"

func main() {
	a := C.adder_t{
		a: 1,
		b: 2,
	}
	fmt.Printf("1 + 2 = %d\n", C.adder_add(&a))
}
```

Or symbols from included headers.

```
package main

/*
#include <math.h>
*/
import "C"
import "fmt"

func main() {
	fmt.Println(C.pow(2, 8))
}
```

## Arrays and slices

Go slices have a [memory layout](https://go.dev/blog/slices-intro) that makes them unique from C arrays. To pass them to and from C, we need to access the underlying slice data, not just the header containing information like the slice's size and capacity.

Historically, there have been [many tricks](https://github.com/golang/go/issues/53003) for accessing and modifying the memory of strings and slices. This post will stick to the newer APIs defined in the unsafe package, but you may see older variants in the wild.

To pass a Go slice as an array of values to C, use [`unsafe.SliceData()`](https://pkg.go.dev/unsafe#SliceData) to get a pointer to the start of the slice's data.

```
package main

/*
#include <stddef.h>

typedef struct person {
	int age;
} person_t;

float average_age(person_t* people, size_t size) {
	int sum = 0;
	for (int i = 0; i < size; i++) {
		sum = sum + people[i].age;
	}
	return (float)sum / size;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func main() {
	people := []C.person_t{
		{age: 10},
		{age: 32},
		{age: 57},
		{age: 92},
	}

	// Pass a Go slice as a C array.
	ave := C.average_age(unsafe.SliceData(people), C.size_t(len(people)))
	fmt.Printf("Average age: %f\n", ave)
}
```

To go the other direction, C arrays can be converted to Go slices using [`unsafe.Slice()`](https://pkg.go.dev/unsafe#Slice). Be careful since this backs the Go slice with the C data, it doesn't copy. So modifying the created slice values modifies the C memory.

```
package main

/*
#include <stddef.h>

typedef struct person {
	int age;
} person_t;

const person_t PEOPLE[] = {
	{.age = 10},
	{.age = 32},
	{.age = 57},
	{.age = 92},
};

const person_t* get_people(size_t* size) {
	*size = 4;
	return PEOPLE;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func main() {
	var size C.size_t
	peoplePtr := C.get_people(&size)

	// Create a Go slice backed by a C array.
	people := unsafe.Slice(peoplePtr, size)

	sum := 0.0
	for _, p := range people {
		sum += float64(p.age)
	}
	fmt.Printf("Average age: %f\n", sum/float64(size))
}
```

## Strings

The "C" package provides a number of convinences for converting between Go strings and C strings, handling the difference in null-termination. 

To convert a Go string to C, use `C.CString()`. Note that this allocates C memory for the resulting string, and must be manually free'd.

```
package main

/*
#include <stdio.h>
#include <stdlib.h>

void say_hello(char* name) {
	printf("Hello, %s!\n", name);
}
*/
import "C"
import "unsafe"

func main() {
	name := C.CString("cgo")
	defer C.free(unsafe.Pointer(name)) // Don't forget to free!

	C.say_hello(name)
}
```

The corresponding `C.GoString()` provides the conversion in the opposite direction, from C to Go.

```
package main

/*
const char* go_proverb() {
	return "With the unsafe package there are no guarantees.";
}
*/
import "C"
import "fmt"

func main() {
	proverb := C.GoString(C.go_proverb())
	fmt.Println(proverb)
}
```

## Pointer fields

Go's runtime implements both garbage collection and stack resizing, routinely moving or reclaiming allocations. Without bookkeeping, there's a danger that Go might garbage collect or move a pointer that's actively being used by C. 

Go pointers used as arguments to C functions are implicitly handled by the Go runtime for the lifetime of the C call (which is why `C.adder_add(&a)` works in the first section). But what if we pass a pointer some other way?

Consider the following program, which takes a struct with a pointer field.

```
package main

/*
#include <stdio.h>

typedef struct person {
	int age;
} person_t;

typedef struct pet {
	int age;
	person_t* owner; // Pointer to a person_t struct.
} pet_t;

void print_pet(pet_t* p) {
	printf("Pet age %d owned by person age %d\n", p->age, p->owner->age);
}
*/
import "C"

func main() {
	person := C.person_t{
		age: 50,
	}
	pet := C.pet_t{
		age:   3,
		owner: &person, // Invalid: passing Go pointer to C without explicit declaration.
	}
	C.print_pet(&pet)
}
```

Go detects that the "person" pointer has been passed to C without handling by the Go runtime, and crashes the program.

```
panic: runtime error: cgo argument has Go pointer to unpinned Go pointer

goroutine 1 [running]:
main.main.func1(0xc000016060)
	.../pointer.go:33 +0x26
main.main()
	.../pointer.go:33 +0x5f
exit status 2
```

There are two potential fixes to this issue.

First, we can allocate the struct using C, sidestepping the Go runtime altogether. Remember to manually free the the returned value.


```
func main() {
	// Person is now allocated and managed by the C runtime, not Go.
	//
	// The "C" package provides "C.sizeof_TYPE" variables for convenience.
	person := (*C.person_t)(C.malloc(C.sizeof_person_t))
	defer C.free(unsafe.Pointer(person)) // Remember to free after use!
	person.age = 50

	pet := C.pet_t{
		age:   3,
		owner: person, // Pointer is managed by C, not Go.
	}
	C.print_pet(&pet)
}
```

Second, Go 1.21 introduced the [`runtime.Pinner`](https://pkg.go.dev/runtime#Pinner) API, which can mark pointers that shouldn't be moved or reclaimed. In this case, Go manages the pointer, but knows not to modify it until after our program calls `Unpin()`.

```
func main() {
	pinner := &runtime.Pinner{}
	defer pinner.Unpin()

	person := C.person_t{
		age: 50,
	}
	pinner.Pin(&person) // "Pin" pointer until Unpin() is called.

	pet := C.pet_t{
		age:   3,
		owner: &person, // Pinned Go pointer is safe to be passed to C.
	}
	C.print_pet(&pet)
}
```

There are many, many more rules and nuances covered by the [cgo docs](https://pkg.go.dev/cmd/cgo#hdr-Passing_pointers).

## errno

One last convenience to mention is Go's automatic handling of errno. Because goroutines can be moved to different OS threads at any time, it's [not safe](https://github.com/golang/go/issues/1360#issuecomment-66053758) to call a C function then read the value of `C.errno` after. Those statements may happen on different threads.

Instead, Go provides automatic handling of errno by allowing an error to be returned from all C calls. The following code calls `getrlimit(2)` and uses the returned error to check for a non-zero errno.

```
package main

/*
#include <sys/resource.h>
typedef struct rlimit rlimit_t; // Create typedef to reference from Go.
*/
import "C"
import (
	"fmt"
	"log"
)

func main() {
	rlimit := C.rlimit_t{}
	if _, err := C.getrlimit(C.RLIMIT_NOFILE, &rlimit); err != nil {
		// errno returned as a Go error.
		log.Fatalf("getrlimit: %v", err)
	}
	fmt.Println("Current:", rlimit.rlim_cur)
	fmt.Println("Maximum:", rlimit.rlim_max)

}
```

It's worth noting this applies to all libraries that implement similar error handling. If you do need to capture a thread specific variable, do so within C.

```
/*
#include <errno.h>
#include <string.h>
#include <sys/resource.h>

typedef struct rlimit rlimit_t;

// Wrapper for getrlimit that also captures errno from the same thread.
int _getrlimit(int resource, struct rlimit *rlim) {
 	int n;
 	n = getrlimit(resource, rlim);
 	if (n == 0) {
 		return 0;
 	}
 	return errno;
}
*/
import "C"
```

## Go from C?

Go _techinically_ provides support for operating as a shared library with [exported C symbols](https://pkg.go.dev/cmd/cgo#hdr-C_references_to_Go).

Practically, because Go shared libraries contain the entire Go runtime, there are some operational drawbacks when loading one. Some querky, such as [lack of `dlclose` support](https://github.com/golang/go/issues/11100). Some catastrophic, like [deadlocks if the loading process ever calls `fork()`](https://github.com/golang/go/issues/15538).

To load a Go shared library safely, you have to control the loading C in program as well. At which point there are undoubtly simplier architectures. And if you don't control the loading C program, maybe use Rust instead?

## _"Cgo is not Go"_

While there are situations where cgo is necessary, it remains an extremely awkward way to write code. C and Go interact in subtle ways that require a considerable amount of manual tracking. Forget to free a variable, use the wrong unsafe method, or make assumptions about what thread you're running on, and you end up with hard to debug memory corruption failure rather than a compile error or stack trace. 

At the end of the day, as Rob Pike's said, [_"Cgo is not Go."_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=12m37s)
