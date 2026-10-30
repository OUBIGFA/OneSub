using System.Text;
using System.Text.Json;

Console.InputEncoding = Encoding.UTF8;
Console.OutputEncoding = new UTF8Encoding(false);
var json = new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };
string requestId = "";
try
{
    // Control messages contain paths only. Large documents are exchanged as artifacts.
    // Errors of this worker are stable E_ codes; the desktop says them in its language.
    var chars = new char[1024 * 1024 + 1];
    var length = Console.In.ReadBlock(chars, 0, chars.Length);
    if (length > 1024 * 1024) throw new InvalidDataException("E_CONTROL_SIZE");
    var request = JsonSerializer.Deserialize<Request>(chars.AsSpan(0, length), json)
        ?? throw new InvalidDataException("E_PROTOCOL");
    requestId = request.RequestId;
    if (request.Protocol != 1) throw new InvalidDataException("E_PROTOCOL");
    if (File.Exists(request.OutputPath)) throw new IOException("E_OUTPUT_EXISTS");
    if (new FileInfo(request.InputPath).Length > 64 * 1024 * 1024)
        throw new InvalidDataException("E_INPUT_SIZE");
    string result = request.Operation switch
    {
        "read" => JsonSerializer.Serialize(Codec.Read(request.InputPath), json),
        "write" => Codec.Write(JsonSerializer.Deserialize<Document>(
            File.ReadAllText(request.InputPath, Encoding.UTF8), json)
            ?? throw new InvalidDataException("E_PROTOCOL")),
        "select-lines" => JsonSerializer.Serialize(Codec.SelectLines(JsonSerializer.Deserialize<LineSelection>(
            File.ReadAllText(request.InputPath, Encoding.UTF8), json)
            ?? throw new InvalidDataException("E_PROTOCOL")), json),
        _ => throw new InvalidDataException("E_PROTOCOL")
    };
    using (var output = new FileStream(request.OutputPath, FileMode.CreateNew, FileAccess.Write))
    {
        var bytes = new UTF8Encoding(false, true).GetBytes(result);
        output.Write(bytes);
        output.Flush(true);
    }
    Console.WriteLine(JsonSerializer.Serialize(new { protocol = 1, requestId, ok = true }, json));
}
catch (Exception error)
{
    Console.WriteLine(JsonSerializer.Serialize(new { protocol = 1, requestId, ok = false, error = error.Message }, json));
    Environment.ExitCode = 1;
}

record Request(int Protocol, string RequestId, string Operation, string InputPath, string OutputPath);
record LineSelection(Document Document, string Line);
