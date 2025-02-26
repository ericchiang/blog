+++
title = "Protobuf generators for fun and profit"
date = "2025-02-25"
+++

There aren't a ton of great options for API definition languages. OpenAPI and gRPC are the only real game in town, but each have their own shortcomings. OpenAPI is a huge, awkward language for expressing types. gRPC is focused on its own protocol rather than interoping with more general semantics like REST.

Since I vastly prefer the Protocol Buffer language over YAML, I was recently looking into [REST bindings][proto-rest] for gRPC, but the inconsistency of the ecosystem stands out. Individual projects run the gambit between totally battle tested and basically unmaintained.

This is a common story. If you use protobuf outside of a certain giant cooperation that makes them work for everything internally, it's inevitable that you'll want to generate something that there's not a convenient Open Source project for. OpenAPI v3 specs, hosted documentation, JSON bindings in other languages, etc.

The good news: writing your own protobuf generators is super easy.

[proto-rest]: https://cloud.google.com/blog/products/api-management/bridge-the-gap-between-grpc-and-rest-http-apis

## protoc plugins

Though you can technically [parse a proto file][go-protoparser] directly, it’s much easier to let the Protocol Buffer Compiler (protoc) resolve imports and types for you. protoc plugins then run as a subprocess, receiving a parsed intermediate format through stdin and returning a response through stdout.

[go-protoparser]: https://github.com/yoheimuta/go-protoparser

![](../imgs/protoc_plugins.png)

Amusingly, the request and response types are themselves protobuf messages.

The request includes parsed files passed to the compiler, and a list of files that are expected to be operated on.

```
// https://github.com/protocolbuffers/protobuf/blob/main/src/google/protobuf/compiler/plugin.proto
//
// An encoded CodeGeneratorRequest is written to the plugin's stdin.
message CodeGeneratorRequest {
  // The .proto files that were explicitly listed on the command-line.
  //
  // ...
  repeated string file_to_generate = 1; // Example: ["file.proto"]

  // FileDescriptorProtos for all files in files_to_generate and everything
  // they import.
  // ...
  repeated FileDescriptorProto proto_file = 15;

  // ...
}
```

The plugin then returns generated files for each of the inputs.

```
// The plugin writes an encoded CodeGeneratorResponse to stdout.
message CodeGeneratorResponse {
  // Represents a single generated file.
  message File {
    // The file name, relative to the output directory...
    optional string name = 1; // Example: "file.gen"

    // The file contents.
    optional string content = 15;

    // ...
  }
  repeated File file = 15;
}
```

A simple Go skeleton looks something like the following.

```
package main

import (
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse stdin: %v\n", err)
		os.Exit(1)
	}
	resp, err := generate(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Generating output: %v\n", err)
		os.Exit(1)
	}
	out, err := proto.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Encoding response: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
}

func generate(req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
	// ...
}
```

## Writing a plugin

Let's say we have the following proto file and would like to generate markdown docs to host.

Of course, we don't just want to copy our messages and field names.	Docs should include information from the original definition like comments and type information.

```
edition = "2023";

package io.github.ericchiang;

// A food ingredient to use for a recipe.
message Ingredient {
	// The name of the ingredient.
	string name = 1;
	// The amount required, in grams.
	int64 amount = 2;
}

// A delicious recipe to make!
message Recipe {
	// Name of the recipe. Required.
	string name = 1;
	// A list of ingredients used in the recipe. 
	repeated Ingredient ingredients = 2;
	// A set of steps to follow for the recipe.
	repeated string steps = 3;
}
```

The raw request passed to protoc plugins can be a little clunky. Luckily, Go's protobuf reflection package has conveniences to convert these to reflection objects. This provides easy APIs for looking up files, messages, and comments by names and types. 

The following is a little less than 100 lines of code that iterates through ever message in a file, inspecting every field, and generating documentation including original comments and type information. 

```
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	// ...
}

func generate(req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
	set := &descriptorpb.FileDescriptorSet{File: req.GetProtoFile()}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("parsing files: %v", err)
	}

	resp := &pluginpb.CodeGeneratorResponse{
		// Mark support for newer proto features.
		SupportedFeatures: proto.Uint64(uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)),
		MaximumEdition:    proto.Int32(int32(descriptorpb.Edition_EDITION_2024)),
	}
 
	for _, path := range req.GetFileToGenerate() {
		// For each file, generate a markdown file.
		buf := &bytes.Buffer{}
		desc, err := files.FindFileByPath(path)
		if err != nil {
			return nil, fmt.Errorf("looking up file %s: %v", path, err)
		}

		msgs := desc.Messages()
		for i := range msgs.Len() {
			msg := msgs.Get(i)

			srcs := msg.ParentFile().SourceLocations()
			src := srcs.ByDescriptor(msg)

			// Write the message name and associated comment.
			buf.WriteString("# " + string(msg.Name()) + "\n")
			if comment := strings.TrimSpace(src.LeadingComments); comment != "" {
				buf.WriteString(comment + "\n\n")
			}

			// Generate documentation for each field.
			buf.WriteString("Fields:\n")
			fields := msg.Fields()
			for i := range fields.Len() {
				field := fields.Get(i)
				// Write field name.
				buf.WriteString("* " + string(field.JSONName()) + " - _")

				// Write type information of the field.
				if field.Cardinality() == protoreflect.Repeated {
					buf.WriteString("repeated ")
				}
				kind := field.Kind().String()
				if field.Kind() == protoreflect.MessageKind {
					// If the type is a message, link to that message.
					name := string(field.Message().Name())
					kind = "[" + name + "](#" + strings.ToLower(name) + ")"
				}
				buf.WriteString(kind + "_ - ")

				// Write the field's comment.
				src := srcs.ByDescriptor(field)
				if comment := strings.TrimSpace(src.LeadingComments); comment != "" {
					comment = strings.ReplaceAll(comment, "\n", " ")
					buf.WriteString(comment + "\n")
				}
			}
			buf.WriteString("\n")
		}

		// Replace extension with ".md".
		name, _, _ := strings.Cut(path, ".")
		name += ".md"

		// Return a file to protoc to write.
		file := &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String(name),
			Content: proto.String(buf.String()),
		}
		resp.File = append(resp.File, file)
	}
	return resp, nil
}
```

Finally, compile to a binary with the name `protoc-gen-{NAME}`, add the binary to your `PATH`, and run protoc with the flag `--{NAME}_out={OUTDIR}`.

```
go build -o protoc-gen-markdown main.go
PATH="${PATH}:." protoc --markdown_out=. food.proto
```

protoc invokes the plugin and writes the generated markdown file to the output directory.

```
% cat food.md 
# Ingredient
A food ingredient to use for a recipe.

Fields:
* name - _string_ - The name of the ingredient.
* amount - _int64_ - The amount required, in grams.

# Recipe
A delicious recipe to make!

Fields:
* name - _string_ - Name of the recipe. Required.
* ingredients - _repeated [Ingredient](#ingredient)_ - A list of ingredients used in the recipe.
* steps - _repeated string_ - A set of steps to follow for the recipe.

```

## ...profit?

It's worth mentioning how good the the newer(ish) [Go protobuf APIs][go-proto] are. Particularly the reflection capabilities. There was a [kerfuffle][go-proto-hn] around import paths when they were announced, but the packages are a huge upgrade. [`google.golang.org/protobuf/reflect/protodesc`][protodesc] is a superb tool for plugin authors.

With regards to protobuf itself. While it'd be amazing if everything worked perfectly out of the box, an easy plugin system makes betting on protobuf much more palatable. If there are missing OSS projects, you can always program your way out in a pinch. 

[go-proto]: https://go.dev/blog/protobuf-apiv2
[go-proto-hn]:  https://news.ycombinator.com/item?id=22468494
[protodesc]: https://google.golang.org/protobuf/reflect/protodesc
