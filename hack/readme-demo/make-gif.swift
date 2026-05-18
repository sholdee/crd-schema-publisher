import AppKit
import Foundation
import ImageIO
import UniformTypeIdentifiers

if CommandLine.arguments.count != 3 {
    fputs("usage: make-gif <frames.tsv> <output.gif>\n", stderr)
    exit(2)
}

let manifestURL = URL(fileURLWithPath: CommandLine.arguments[1])
let outputURL = URL(fileURLWithPath: CommandLine.arguments[2])
let manifest = try String(contentsOf: manifestURL, encoding: .utf8)
let frames = manifest
    .split(separator: "\n")
    .map { line -> (String, Double) in
        let parts = line.split(separator: "\t")
        guard parts.count == 2, let delay = Double(parts[1]) else {
            fatalError("invalid frame manifest line: \(line)")
        }
        return (String(parts[0]), delay)
    }

guard let destination = CGImageDestinationCreateWithURL(
    outputURL as CFURL,
    UTType.gif.identifier as CFString,
    frames.count,
    nil
) else {
    fatalError("could not create GIF destination")
}

let gifProperties: [CFString: Any] = [
    kCGImagePropertyGIFDictionary: [
        kCGImagePropertyGIFLoopCount: 0,
    ],
]
CGImageDestinationSetProperties(destination, gifProperties as CFDictionary)

for (path, delay) in frames {
    let url = URL(fileURLWithPath: path)
    guard let image = NSImage(contentsOf: url) else {
        fatalError("could not load frame: \(path)")
    }

    var rect = NSRect(origin: .zero, size: image.size)
    guard let cgImage = image.cgImage(forProposedRect: &rect, context: nil, hints: nil) else {
        fatalError("could not create CGImage for frame: \(path)")
    }

    let frameProperties: [CFString: Any] = [
        kCGImagePropertyGIFDictionary: [
            kCGImagePropertyGIFDelayTime: delay,
        ],
    ]
    CGImageDestinationAddImage(destination, cgImage, frameProperties as CFDictionary)
}

if !CGImageDestinationFinalize(destination) {
    fatalError("could not finalize GIF")
}
