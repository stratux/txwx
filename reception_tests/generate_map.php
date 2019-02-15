<?php
/*::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::*/
/*::                                                                         :*/
/*::  This routine calculates the distance between two points (given the     :*/
/*::  latitude/longitude of those points). It is being used to calculate     :*/
/*::  the distance between two locations using GeoDataSource(TM) Products    :*/
/*::                                                                         :*/
/*::  Definitions:                                                           :*/
/*::    South latitudes are negative, east longitudes are positive           :*/
/*::                                                                         :*/
/*::  Passed to function:                                                    :*/
/*::    lat1, lon1 = Latitude and Longitude of point 1 (in decimal degrees)  :*/
/*::    lat2, lon2 = Latitude and Longitude of point 2 (in decimal degrees)  :*/
/*::    unit = the unit you desire for results                               :*/
/*::           where: 'M' is statute miles (default)                         :*/
/*::                  'K' is kilometers                                      :*/
/*::                  'N' is nautical miles                                  :*/
/*::  Worldwide cities and other features databases with latitude longitude  :*/
/*::  are available at https://www.geodatasource.com                          :*/
/*::                                                                         :*/
/*::  For enquiries, please contact sales@geodatasource.com                  :*/
/*::                                                                         :*/
/*::  Official Web site: https://www.geodatasource.com                        :*/
/*::                                                                         :*/
/*::         GeoDataSource.com (C) All Rights Reserved 2018                  :*/
/*::                                                                         :*/
/*::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::*/
	function distance($lat1, $lon1, $lat2, $lon2, $unit) {
		if (($lat1 == $lat2) && ($lon1 == $lon2)) {
			return 0;
		} else {
			$theta = $lon1 - $lon2;
			$dist = sin(deg2rad($lat1)) * sin(deg2rad($lat2)) +  cos(deg2rad($lat1)) * cos(deg2rad($lat2)) * 	cos(deg2rad($theta));
			$dist = acos($dist);
			$dist = rad2deg($dist);
			$miles = $dist * 60 * 1.1515;
			$unit = strtoupper($unit);
	
			if ($unit == "K") {
				return ($miles * 1.609344);
			} else if ($unit == "N") {
				return ($miles * 0.8684);
			} else {
				return $miles;
			}
		}
	}

	if (isset($_FILES['messages_log'])){
		$s = file_get_contents($_FILES['messages_log']['tmp_name']);
	
		$x = explode("\n", $s);
	
		$tx_moving = isset($_GET['tx_moving']) ? $_GET['tx_moving'] == 1 : false;
	
		$fixed_lat = 0;
		$fixed_lng = 0;
	
		$max_dist = 0;
		$max_dist_lat = 0;
		$max_dist_lng = 0;
	
		$hits = array();
	
		$s = "";
	
		foreach ($x as $a) {
			$x2 = explode(',', $a);
			if ($fixed_lat == 0) {
				if ($tx_moving) {
					if ($x2[2] != 0) {
						$fixed_lat = $x2[2];
						$fixed_lng = $x2[3];
						$s .= "addMarker(${fixed_lat}, ${fixed_lng}, 'RX', 'Low');\n";
					}
				} else {
					if ($x2[0] != 0) {
						$fixed_lat = $x2[0];
						$fixed_lng = $x2[1];
						$s .= "addMarker(${fixed_lat}, ${fixed_lng}, 'TX', 'Low');\n";
					}
				}
			}
	
			if ($tx_moving && ($x2[0] != 0)) {
				$s .= "addDot(" . $x2[0] . "," . $x2[1] . ",'0', true);\n";
				$hits[] = array($x2[0], $x2[1]);
				$d = distance($fixed_lat, $fixed_lng, $x2[0], $x2[1], 'K');
				if ($d > $max_dist) {
					$max_dist = $d;
					$max_dist_lat = $x2[0];
					$max_dist_lng = $x2[1];
				}
			} else if (!$tx_moving && ($x2[2] != 0)) {
				$s .= "addDot(" . $x2[2] . "," . $x2[3] . ",'0', true);\n";
				$hits[] = array($x2[2], $x2[3]);
				$d = distance($fixed_lat, $fixed_lng, $x2[2], $x2[3], 'K');
				if ($d > $max_dist) {
					$max_dist = $d;
					$max_dist_lat = $x2[2];
					$max_dist_lng = $x2[3];
				}
			}
		}
	
		$max_dist = sprintf("%0.2f", $max_dist);
		$s .= "addMaxDistLine(${fixed_lat}, ${fixed_lng}, ${max_dist_lat}, ${max_dist_lng}, ${max_dist});\n";
	
		$ss = file_get_contents("index.html");
		$sss = str_replace('DATA_HERE', $s, $ss);
	
		print "$sss\n";
		exit;
	}
?>

<html>
   <body>
      Receiver moving:
      <form action="generate_map.php?tx_moving=0" method="POST" enctype="multipart/form-data">
         <input type="file" name="messages_log" />
         <input type="submit"/>
      </form><br/>
      Transmitter moving:
      <form action="generate_map.php?tx_moving=1" method="POST" enctype="multipart/form-data">
         <input type="file" name="messages_log" />
         <input type="submit"/>
      </form><br/>
      
   </body>
</html>