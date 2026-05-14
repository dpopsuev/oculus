package analyzer

import (
	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/ts"
)

// hasKeywordChild reports whether any direct child (or its first child) of node
// has Content == keyword. Handles both bare keyword tokens and wrapper nodes
// like C# [modifier]→[async] or Kotlin [modifiers]→[function_modifier]→[suspend].
func hasKeywordChild(node ts.Node, src []byte, keyword string) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Content(src) == keyword {
			return true
		}
		// One level deeper (C# modifier wrapper, Kotlin modifiers block)
		for j := 0; j < int(child.ChildCount()); j++ {
			gc := child.Child(j)
			if gc.Content(src) == keyword {
				return true
			}
			// Two levels deeper (Kotlin modifiers → function_modifier → suspend)
			for k := 0; k < int(gc.ChildCount()); k++ {
				if gc.Child(k).Content(src) == keyword {
					return true
				}
			}
		}
	}
	return false
}

// walkAsyncCallees walks a tree and calls onCall for every call_expression-like
// node, passing the function/method name and the node itself.
// nameField is the field name for the callee (e.g. "function", "method").
func walkNodes(n ts.Node, nodeType string, fn func(ts.Node)) {
	if n.Type() == nodeType {
		fn(n)
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		walkNodes(n.Child(i), nodeType, fn)
	}
}

// extractCSharpAsyncCallees detects:
//   - method has [async] modifier → body: await_expression → call
//   - Task.Run(delegate)          → CallEdgeTaskSpawn
func extractCSharpAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		switch n.Type() {
		case "await_expression":
			// C# await expr — [await] [invocation_expression [identifier] [argument_list]]
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "invocation_expression" {
					// callee is first identifier/member_access_expression child
					if fn, ok := child0ByType(child, "identifier", "member_access_expression"); ok {
						name := extractSimpleName(fn, src)
						if name != "" && name != "await" {
							out[name] = oculus.CallEdgeAwait
						}
					}
				}
			}
		case "invocation_expression":
			if fn := n.ChildByFieldName("expression"); fn != nil {
				name := extractSimpleName(fn, src)
				switch name {
				case "Run", "StartNew": // Task.Run, Task.Factory.StartNew
					out[name] = oculus.CallEdgeTaskSpawn
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractSwiftAsyncCallees detects:
//   - await_expression → call_expression  → CallEdgeAwait
//   - Task { }                            → CallEdgeTaskSpawn
func extractSwiftAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		switch n.Type() {
		case "await_expression":
			// Swift: [await]="await" [call_expression [simple_identifier]="fetch" ...]
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "call_expression" {
					// callee is first simple_identifier child
					if fn, ok := child0ByType(child, "simple_identifier", "identifier"); ok {
						if name := fn.Content(src); name != "" {
							out[name] = oculus.CallEdgeAwait
						}
					}
				}
			}
		case "call_expression":
			if fn, ok := child0ByType(n, "simple_identifier", "identifier"); ok {
				switch fn.Content(src) {
				case "Task", "TaskGroup":
					out[fn.Content(src)] = oculus.CallEdgeTaskSpawn
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractKotlinAsyncCallees detects:
//   - function has [suspend] modifier
//   - launch { }, async { }  → CallEdgeTaskSpawn
//   - call inside coroutine  → CallEdgeAwait (Kotlin suspend fns are called directly)
func extractKotlinAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "call_expression" {
			// Get the called name from simple_identifier child
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "simple_identifier" {
					switch child.Content(src) {
					case "launch", "async", "runBlocking", "withContext":
						out[child.Content(src)] = oculus.CallEdgeTaskSpawn
					}
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractJavaAsyncCallees detects:
//   - CompletableFuture.supplyAsync / runAsync → CallEdgeTaskSpawn
//   - thenApply / thenAccept / thenCompose     → CallEdgePromise (continuation)
//   - ExecutorService.submit / execute          → CallEdgeTaskSpawn
func extractJavaAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "method_invocation" {
			name := ""
			// method_invocation: [object].[name](args)
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				name = nameNode.Content(src)
			}
			switch name {
			case "supplyAsync", "runAsync", "submit", "execute":
				out[name] = oculus.CallEdgeTaskSpawn
			case "thenApply", "thenAccept", "thenCompose", "thenRun",
				"exceptionally", "whenComplete", "handle":
				out[name] = oculus.CallEdgePromise
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractCAsyncCallees detects:
//   - pthread_create(_, _, fn, _)  → CallEdgeGoroutine (closest semantic match)
//   - thrd_create (C11)            → CallEdgeGoroutine
func extractCAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "call_expression" {
			if fn := n.ChildByFieldName("function"); fn != nil {
				switch fn.Content(src) {
				case "pthread_create", "thrd_create":
					// Third argument is the thread function — emit a goroutine-like edge.
					if args := n.ChildByFieldName("arguments"); args != nil {
						// args: ( expr, expr, fn_ptr, expr )
						positional := 0
						for i := 0; i < int(args.ChildCount()); i++ {
							arg := args.Child(i)
							if arg.Type() == "," || arg.Type() == "(" || arg.Type() == ")" {
								continue
							}
							positional++
							if positional == 3 { // thread function
								name := extractSimpleName(arg, src)
								if name != "" {
									out[name] = oculus.CallEdgeGoroutine
								}
							}
						}
					}
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractCppAsyncCallees detects:
//   - std::async(fn, ...)     → CallEdgeTaskSpawn
//   - std::thread(fn, ...)    → CallEdgeGoroutine
//   - co_await expr           → CallEdgeAwait (C++20 coroutines)
func extractCppAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		switch n.Type() {
		case "call_expression":
			// C++ call_expression first child is the callee (may be qualified_identifier
			// like std::async — ChildByFieldName("function") may return nil).
			var fnName string
			if fn := n.ChildByFieldName("function"); fn != nil {
				fnName = extractSimpleName(fn, src)
			} else if n.ChildCount() > 0 {
				fnName = extractSimpleName(n.Child(0), src)
			}
			if fnName == "async" {
				// std::async(callable, ...) — find argument_list, emit first arg
				for i := 0; i < int(n.ChildCount()); i++ {
					arg := n.Child(i)
					if arg.Type() == "argument_list" {
						for j := 0; j < int(arg.ChildCount()); j++ {
							a := arg.Child(j)
							if a.Content(src) == "(" || a.Content(src) == ")" || a.Type() == "," {
								continue
							}
							if argName := extractSimpleName(a, src); argName != "" {
								out[argName] = oculus.CallEdgeTaskSpawn
							}
							break
						}
						break
					}
				}
			}
		case "declaration":
			// std::thread t(fn) — C++ most-vexing-parse produces:
			// declaration → [qualified_identifier=std::thread] [function_declarator → identifier=t [parameter_list → parameter_declaration → type_identifier=fn]]
			isThreadDecl := false
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "type_identifier" && child.Content(src) == "thread" {
					isThreadDecl = true
				}
				if child.Type() == "qualified_identifier" && extractSimpleName(child, src) == "thread" {
					isThreadDecl = true
				}
			}
			if isThreadDecl {
				for i := 0; i < int(n.ChildCount()); i++ {
					decl := n.Child(i)
					switch decl.Type() {
					case "function_declarator":
						// std::thread t(fn) — most-vexing parse: fn appears as parameter_declaration
						for j := 0; j < int(decl.ChildCount()); j++ {
							plist := decl.Child(j)
							if plist.Type() == "parameter_list" {
								for k := 0; k < int(plist.ChildCount()); k++ {
									param := plist.Child(k)
									if param.Type() == "parameter_declaration" {
										for l := 0; l < int(param.ChildCount()); l++ {
											if name := param.Child(l).Content(src); name != "" {
												out[name] = oculus.CallEdgeGoroutine
											}
										}
									}
								}
							}
						}
					case "init_declarator":
						// std::thread t2(fn, args...) — normal init: first arg of argument_list is fn
						for j := 0; j < int(decl.ChildCount()); j++ {
							arg := decl.Child(j)
							if arg.Type() == "argument_list" {
								for k := 0; k < int(arg.ChildCount()); k++ {
									a := arg.Child(k)
									if a.Content(src) == "(" || a.Content(src) == ")" || a.Type() == "," {
										continue
									}
									if name := extractSimpleName(a, src); name != "" {
										out[name] = oculus.CallEdgeGoroutine
									}
									break
								}
								break
							}
						}
					}
				}
			}
		case "co_await_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "call_expression" {
					if fn := child.ChildByFieldName("function"); fn != nil {
						if name := extractSimpleName(fn, src); name != "" {
							out[name] = oculus.CallEdgeAwait
						}
					}
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// child0ByType returns the first direct child whose Type() matches any of types,
// and a boolean indicating whether one was found.
func child0ByType(node ts.Node, types ...string) (ts.Node, bool) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		for _, t := range types {
			if child.Type() == t {
				return child, true
			}
		}
	}
	return node, false // return node as dummy; caller checks bool
}
