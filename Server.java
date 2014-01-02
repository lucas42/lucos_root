import java.io.* ;
import java.net.* ;
import java.util.* ; 
import java.lang.* ; 
import com.google.gson.*;
import java.math.BigInteger;
final class Server implements Runnable {
	final static String CRLF = "\r\n";
	Socket socket;
	String fullHostname;
	String hostname;
	DataOutputStream os;
	OutputStreamWriter osw;
	Map<String, String> header = new HashMap<String, String>();
	Map<String, String> get = new HashMap<String, String>();
	static Queue<Resource> resourceList = new LinkedList<Resource>();
	static Gson gson = new Gson();
	
	// Constructor
	public Server(Socket socket) throws Exception {
		this.socket = socket;
		fullHostname = socket.getInetAddress().getHostName();
	}
	
	// Implement the run() method of the Runnable interface.
	public void run() {
		try {
			processRequest();
		} catch (Exception e) {
			System.err.println("Server Error (HttpRequest, host:"+fullHostname+"):");
			e.printStackTrace();
		}

	}
	private void processRequest() throws Exception {
	
		// Get a reference to the socket's input and output streams.
		InputStream is = new DataInputStream(socket.getInputStream());
		os = new DataOutputStream(socket.getOutputStream());
		osw = new OutputStreamWriter(os, "UTF8");
		BufferedReader br = new BufferedReader(new InputStreamReader(is, "UTF8"));

		String requestLine = br.readLine();

		// Get the header lines.
		String headerLine = null;
		while (br.ready()) {
			headerLine = br.readLine();
			if (headerLine.length() < 1) break;
			int jj = headerLine.indexOf(':');
			String field;
			if ( jj > -1) {
				field = headerLine.substring(jj+1);
				headerLine = headerLine.substring(0, jj);
			} else {
				field = "true";
			}
			header.put(headerLine, field);
		}
		
		// Extract the filename from the request line.
		StringTokenizer tokens = new StringTokenizer(requestLine);

		String method = tokens.nextToken().toLowerCase();
		String path = tokens.nextToken();
		path.trim();
		String fullpath = path;
		int ii = path.indexOf('?');
		if (ii > -1) {
			String[] getstring = path.substring(ii+1).split("&");
			for (String key : getstring) {
				int jj = key.indexOf('=');
				String field;
				if ( jj > -1) {
					field = key.substring(jj+1);
					key = key.substring(0, jj);
				} else {
					field = "true";
				}
				key = URLDecoder.decode(key, "UTF-8");
				field = URLDecoder.decode(field, "UTF-8");
				get.put(key, field);
			}
			path = path.substring(0, ii);
		}
		if (path.equals("/time") && method.equals("get")) {
			Date date = new Date();
			sendHeaders(200, "OK", "application/json");
			osw.write(Long.toString(date.getTime())); 
		} else if(path.equals("/resources")) {
			Map<String, Object> output = new HashMap<String, Object>();
			Map<String, String> resources = new HashMap<String, String>();
			long version;
			try {
				version = Long.parseLong(get.get("v"));
			} catch (NumberFormatException e) {
				version = 0;
			}
			long newVersion = version;
			
			Iterator resourceiter = resourceList.iterator();
			while (resourceiter.hasNext()) {
				Resource resource = (Resource) resourceiter.next();
				try {
					resources.put(resource.key, resource.type);
					File file = new File(resource.fileName);
					if (file.lastModified() > version) {
						if (file.lastModified() > newVersion) newVersion = file.lastModified();
						String content = readFile(resource.fileName);
						
						output.put(resource.key, content);
					}
				} catch (FileNotFoundException e) {
				}
			}
			output.put("v", newVersion);
			output.put("r", resources);
			sendHeaders(200, "OK", "application/json");
			osw.write(gson.toJson(output)); 
		} else if(path.equals("/")) {
			String content = "";
			try {
				content = readFile("index.xhtml");
				// TODO: read modules.json and insert any modules which don't require JS into the HTML
			} catch (FileNotFoundException e) {
			}
			sendHeaders(200, "OK", "application/xhtml+xml");
			osw.write(content); 
		} else if(path.equals("/reading")) {
			String content = "";
			try {
				content = readFile("reading.xhtml");
			} catch (FileNotFoundException e) {
			}
			sendHeaders(200, "OK", "application/xhtml+xml");
			osw.write(content); 
		} else {
			String fileName = null;
			if (method.equals("get")) {
				path = path.replaceAll("/\\.\\./","");
				if (path.charAt(0) != '/') path = "/" + path;
				if (path.equals("/script")) fileName = "../core/lucos.js";
				else if (path.equals("/bootloader")) fileName = "../core/bootloader.js";
				else if (path.equals("/lucos.manifest")) fileName = "./manifest";
				else if (path.equals("/icon")) fileName = "./icon.png";
				else if (path.equals("/favicon.ico")) fileName = "./favicon.png";
				else fileName = "." + path;
				if (fileName.charAt(fileName.length()-1) == '/') fileName += "index.xhtml";
			}
			
			// Open the requested file.
			FileInputStream fis = null;
			boolean fileExists = true;
			String statusLine = null;
			try {
				fis = new FileInputStream(fileName);
				 statusLine = "HTTP/1.1 200 OK";
			} catch (FileNotFoundException e) {
				System.err.println("File Not found: "+requestLine);
				fileName = "./404.html";
				statusLine = "HTTP/1.1 404 File Not Found";
				try {
					fis = new FileInputStream(fileName);
				} catch (FileNotFoundException e2) {
					fileExists = false;
				}
			}



			// Construct the response message.
			String contentTypeLine = null;
			String entityBody = null;
			if (fileExists) {
				contentTypeLine = "Content-Type: " +
					contentType( fileName ) + "; charset=UTF-8";
			}
			// Send the status line.
			os.writeBytes(statusLine + CRLF);
			// Send the content type line.
			if (contentTypeLine != null) os.writeBytes(contentTypeLine + CRLF);
			// Send a blank line to indicate the end of the header lines.
			os.writeBytes(CRLF);

			// Send the entity body.
			if (fileExists) {
				 sendBytes(fis);
				 fis.close();
			} else {
				 os.writeBytes("error: 404 file not found");
			}
		}
		
		// Close streams and socket.
		osw.close();
		br.close();
		socket.close();

	}

	private String readFile(String fileName) throws Exception {
		FileInputStream fis = new FileInputStream(fileName);
		int bytes = 0;
		StringBuffer contentBuffer = new StringBuffer("");
		while((bytes = fis.read()) != -1) contentBuffer.append((char)bytes);
		fis.close();
		return contentBuffer.toString();
	}

	private void sendBytes(FileInputStream fis) throws Exception {
	   // Construct a 1K buffer to hold bytes on their way to the socket.
	   byte[] buffer = new byte[1024];
	   int bytes = 0;
	   
	   // Copy requested file into the socket's output stream.
	   while((bytes = fis.read(buffer)) != -1 ) {
		  os.write(buffer, 0, bytes);
	   }
	}

	private static String contentType(String fileName) {
		if(fileName.endsWith(".htm") || fileName.endsWith(".html")) {
			return "text/html";
		}
		if(fileName.endsWith(".xhtml")) {
			return "application/xhtml+xml";
		}
		if(fileName.endsWith(".png")) {
			return "image/png";
		}
		if(fileName.endsWith(".gif")) {
			return "image/gif";
		}
		if(fileName.endsWith(".jpg")) {
			return "image/jpeg";
		}
		if(fileName.endsWith(".css")) {
			return "text/css";
		}
		if(fileName.endsWith(".js")) {
			return "text/javascript";
		}
		if(fileName.endsWith(".txt")) {
			return "text/plain";
		}
		if(fileName.endsWith("manifest")) {
			return "text/cache-manifest";
		}
		return "application/octet-stream";
	}
	private float getFloat(String param) throws IOException {
		try{
			float value = Float.parseFloat(get.get(param));
			return value;
		} catch (NumberFormatException e) {
			sendHeaders(400, "Not Changed", "application/json");
			osw.write(param + " must be a number");
		} catch (NullPointerException e) {
			sendHeaders(400, "Not Changed", "application/json");
			osw.write(param + " must be set");
		}
		return -1;
	}
	private void sendHeaders(int status, String statusstring, Map<String, String> extraheaders) throws IOException {
		os.writeBytes("HTTP/1.1 "+ status +" "+ statusstring + CRLF);
		os.writeBytes("Access-Control-Allow-Origin: *" + CRLF);
		os.writeBytes("Server: lucos" + CRLF);
		Iterator iter = extraheaders.entrySet().iterator();
		while (iter.hasNext()) {
			Map.Entry header = (Map.Entry)iter.next();
			os.writeBytes(header.getKey()+": "+header.getValue() + CRLF);
		}
		os.writeBytes(CRLF);
	}
	private void sendHeaders(int status, String statusstring, String contentType) throws IOException {
		HashMap<String, String> headers =  new HashMap<String, String>();
		headers.put("Content-type", contentType+ "; charset=utf-8");
		sendHeaders(status, statusstring, headers);
	}
	private void redirect(String url) throws IOException {
		HashMap<String, String> headers =  new HashMap<String, String>();
		headers.put("Location", url);
		sendHeaders(302, "Redirect", headers);
	}
	public static void main(String argv[]) throws Exception {
		
		// Set the port number.
		int port = 8003;
		
		// Establish the listen socket.
		ServerSocket serverSocket = new ServerSocket(port);
		System.out.println("outgoing data server ready on port " + port);
		
	
		new Resource("../core/lucos.js", "_lucosjs", "js");
		new Resource("modules.json", "modules", "json");
		new Resource("root.js", "rootjs", "js");
		new Resource("style.css", "style", "css");
    
		// Process HTTP service requests in an infinite loop.
		while (true) {
			// Listen for a TCP connection request.

			Socket clientSocket = serverSocket.accept();
			PrintWriter out = new PrintWriter(clientSocket.getOutputStream(), true);
			// Construct an object to process the HTTP request message.
			Server request = new Server( clientSocket );
			// Create a new thread to process the request.
			Thread thread = new Thread(request);
			// Start the thread.
			thread.start();

		}
	
	}
}

class Resource {
	public String fileName;
	public String key;
	public String type;
	public Resource(String fileName, String key, String type) {
		this.fileName = fileName;
		this.key = key;
		this.type = type;
		Server.resourceList.add(this);
	}
}
