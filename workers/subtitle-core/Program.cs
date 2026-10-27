using System.Text;
using System.Text.Json;

Console.InputEncoding = Encoding.UTF8;
Console.OutputEncoding = new UTF8Encoding(false);
var json = new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };
string requestId = "";
try
{
    // Control messages contain paths only. Large documents are exchanged as artifacts.
    var chars = new char[1024 * 1024 + 1];
    var length = Console.In.ReadBlock(chars, 0, chars.Length);
    if (length > 1024 * 1024) throw new InvalidDataException("控制消息超过 1 MiB");
    var request = JsonSerializer.Deserialize<Request>(chars.AsSpan(0, length), json)
        ?? throw new InvalidDataException("缺少请求");
    requestId = request.RequestId;
    if (request.Protocol != 1) throw new InvalidDataException("不支持的工作进程协议");
    if (File.Exists(request.OutputPath)) throw new IOException("输出文件已存在");
    if (new FileInfo(request.InputPath).Length > 64 * 1024 * 1024)
        throw new InvalidDataException("字幕文件超过 64 MiB");
    string result = request.Operation switch
    {
        "read" => JsonSerializer.Serialize(Codec.Read(request.InputPath), json),
        "write" => Codec.Write(JsonSerializer.Deserialize<Document>(
            File.ReadAllText(request.InputPath, Encoding.UTF8), json)
            ?? throw new InvalidDataException("缺少字幕文档")),
        _ => throw new InvalidDataException("未知操作")
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
