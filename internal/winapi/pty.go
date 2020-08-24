package winapi

// HRESULT WINAPI CreatePseudoConsole(
//     _In_ COORD size,
//     _In_ HANDLE hInput,
//     _In_ HANDLE hOutput,
//     _In_ DWORD dwFlags,
//     _Out_ HPCON* phPC
// );
//
//sys CreatePseudoConsole(size uintptr, hInput windows.Handle, hOutput windows.Handle, dwFlags uint32, phPC *windows.Handle) (hr error) = kernel32.CreatePseudoConsole

// HRESULT WINAPI ResizePseudoConsole(
//     _In_ HPCON hPC,
//     _In_ COORD size
// );
//
//sys ResizePseudoConsole(hPC windows.Handle, size uintptr) (hr error) = kernel32.ResizePseudoConsole

// void WINAPI ClosePseudoConsole(
//     _In_ HPCON hPC
// );
//
//sys ClosePseudoConsole(hPC windows.Handle) (err error) = kernel32.ClosePseudoConsole
